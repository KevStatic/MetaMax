package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/shreejaykurhade/MetaMax/backend/internal/groq"
)


// StackComponent is a detected technology component.
type StackComponent struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"` // "frontend" | "backend" | "database" | "smart-contract" | "infra"
}

// ContainerSpec is a prescription for a single container.
type ContainerSpec struct {
	Name     string            `json:"name"`
	Image    string            `json:"image"`
	Ports    []string          `json:"ports"`
	RAMMb    int64             `json:"ram_mb"`
	CPUCores float64           `json:"cpu_cores"`
	EnvVars  map[string]string `json:"env_vars"`
}

// DetectedOption is a single runnable application found in the repository.
// The /deploy picker offers these to the user before a session starts, so the
// field names here must stay in sync with DetectedOption in the frontend.
type DetectedOption struct {
	Framework  string `json:"framework"`
	Type       string `json:"type"`
	Port       int    `json:"port"`
	InstallCmd string `json:"install_cmd"`
	BuildCmd   string `json:"build_cmd,omitempty"`
	StartCmd   string `json:"start_cmd"`
	SubDir     string `json:"sub_dir,omitempty"`
}

// DeploymentPlan is the output of a repository scan.
type DeploymentPlan struct {
	RepoURL              string           `json:"repo_url"`
	DetectedStack        []StackComponent `json:"detected_stack"`
	Options              []DetectedOption `json:"options"`
	Containers           []ContainerSpec  `json:"containers"`
	HasSmartContracts    bool             `json:"has_smart_contracts"`
	RecommendedNetwork   string           `json:"recommended_network"`
	EstimatedCostPerHour float64          `json:"estimated_cost_per_hour"`
	Summary              string           `json:"summary"`
	DeploymentSteps      []string         `json:"deployment_steps"`
}

// Configured reports whether the scanner has the Groq credentials it needs.
// Without them a scan cannot run and callers should degrade rather than fail.
func (s *Scanner) Configured() bool { return s != nil && s.apiKey != "" }

// Scanner uses Groq to analyze repos and produce deployment plans.
type Scanner struct {
	apiKey string
	model  string
}

// New returns a Scanner backed by the Groq API (OpenAI-compatible).
func New(apiKey, model string) *Scanner {
	if model == "" {
		model = "openai/gpt-oss-120b"
	}
	return &Scanner{apiKey: apiKey, model: model}
}

// AnalyzeRepo clones the repo and returns a deployment plan.
// CloneTimeout bounds the git clone so one unreachable or enormous repository
// cannot pin a request open indefinitely.
const CloneTimeout = 90 * time.Second

// ValidateRepoURL rejects anything that is not a plain http(s) repository URL.
// git treats a leading "-" as a flag, and schemes like file://, ssh:// and
// ext:: can reach the host filesystem or run commands, so only http(s) with a
// real host is allowed through to the clone.
func ValidateRepoURL(raw string) (string, error) {
	clean := sanitizeGitHubURL(raw)
	if clean == "" {
		return "", fmt.Errorf("repo_url is required")
	}
	if strings.HasPrefix(clean, "-") {
		return "", fmt.Errorf("invalid repo URL")
	}
	u, err := url.Parse(clean)
	if err != nil {
		return "", fmt.Errorf("invalid repo URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("repo URL must be http(s)")
	}
	if u.Host == "" {
		return "", fmt.Errorf("repo URL must include a host")
	}
	return clean, nil
}

func (s *Scanner) AnalyzeRepo(ctx context.Context, repoURL string) (*DeploymentPlan, error) {
	repoURL, err := ValidateRepoURL(repoURL)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "comput3-scan-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	cloneCtx, cancel := context.WithTimeout(ctx, CloneTimeout)
	defer cancel()

	cloneURL := extractRepoRoot(repoURL)
	// "--" stops git parsing any later argument as a flag.
	cmd := exec.CommandContext(cloneCtx, "git", "clone", "--depth=1", "--quiet", "--", cloneURL, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		if cloneCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("clone %s: timed out after %s", repoURL, CloneTimeout)
		}
		return nil, fmt.Errorf("clone %s: %w\n%s", repoURL, err, string(out))
	}

	files, err := collectFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("collect files: %w", err)
	}

	return s.analyzeWithGroq(ctx, repoURL, files)
}

type repoFile struct {
	Path    string
	Content string
}

func collectFiles(root string) ([]repoFile, error) {
	priority := []string{
		"package.json", "requirements.txt", "pyproject.toml", "go.mod",
		"Dockerfile", "docker-compose.yml", "docker-compose.yaml",
		"hardhat.config.js", "hardhat.config.ts", "foundry.toml",
		"next.config.js", "next.config.ts",
		"vite.config.js", "vite.config.ts",
		// Without a manifest for the language it is actually written in, the
		// model is left guessing from the repo name and README — which is how
		// a C# solution gets reported back as a Next.js app.
		"Cargo.toml", "composer.json", "Gemfile", "pom.xml",
		"build.gradle", "build.gradle.kts", "global.json",
		".env.example", ".env.sample", "README.md",
	}

	seen := map[string]bool{}
	var files []repoFile

	for _, name := range priority {
		content, err := readTruncated(filepath.Join(root, name), 4000)
		if err == nil {
			files = append(files, repoFile{Path: name, Content: content})
			seen[name] = true
		}
	}

	solCount := 0
	projCount := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			n := d.Name()
			if strings.HasPrefix(n, ".") || n == "node_modules" ||
				n == "dist" || n == "build" || n == ".next" ||
				n == "out" || n == "target" || n == "artifacts" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if seen[rel] || strings.Count(rel, "/") > 2 {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		base := filepath.Base(path)

		if ext == ".sol" && solCount < 5 {
			if content, err := readTruncated(path, 3000); err == nil {
				files = append(files, repoFile{Path: rel, Content: content})
				seen[rel] = true
				solCount++
			}
		}
		// Project files for the compiled ecosystems live in subdirectories
		// rather than at the root, so they are only reachable from the walk.
		// The .NET ones carry the TargetFramework the image has to match.
		if projCount < 8 {
			switch ext {
			case ".csproj", ".fsproj", ".vbproj", ".sln", ".slnx":
				if content, err := readTruncated(path, 3000); err == nil {
					files = append(files, repoFile{Path: rel, Content: content})
					seen[rel] = true
					projCount++
					return nil
				}
			}
		}
		if base == "package.json" || base == "hardhat.config.js" ||
			base == "hardhat.config.ts" || base == "foundry.toml" ||
			base == "requirements.txt" || base == "go.mod" ||
			base == "pom.xml" || base == "build.gradle" ||
			base == "build.gradle.kts" || base == "Gemfile" ||
			base == "composer.json" || base == "Cargo.toml" {
			if content, err := readTruncated(path, 3000); err == nil {
				files = append(files, repoFile{Path: rel, Content: content})
				seen[rel] = true
			}
		}
		return nil
	})
	return files, err
}

func (s *Scanner) analyzeWithGroq(ctx context.Context, repoURL string, files []repoFile) (*DeploymentPlan, error) {
	var sb strings.Builder
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("\n\n### FILE: %s\n```\n%s\n```", f.Path, f.Content))
	}

	prompt := fmt.Sprintf(`You are a deployment analyzer for MetaMax, a decentralized trustless compute platform for AI agents.

Analyze the repository files below and return a JSON deployment plan.

Repository: %s

Files:
%s

Return ONLY valid JSON matching this exact schema (no markdown, no explanation):
{
  "repo_url": "string",
  "detected_stack": [
    {"name": "string", "version": "string", "type": "frontend|backend|database|smart-contract|infra"}
  ],
  "options": [
    {
      "framework": "Next.js",
      "type": "frontend|backend|fullstack|static",
      "port": 3000,
      "install_cmd": "npm ci",
      "build_cmd": "npm run build",
      "start_cmd": "npm start",
      "sub_dir": "" 
    }
  ],
  "containers": [
    {
      "name": "string",
      "image": "official Docker image tag",
      "ports": ["3000/tcp"],
      "ram_mb": 2048,
      "cpu_cores": 1.0,
      "env_vars": {"KEY": "value or empty string"}
    }
  ],
  "has_smart_contracts": false,
  "recommended_network": "monad-testnet or mainnet or none",
  "estimated_cost_per_hour": 0.05,
  "summary": "one-sentence description",
  "deployment_steps": ["step1", "step2"]
}

Rules:
- "options": one entry per independently runnable app in the repo (a monorepo
  with a web/ and api/ folder yields two). Set sub_dir to the folder holding
  that app, or "" when it is at the repo root. Omit build_cmd when none is
  needed. Return [] only if nothing runnable was found.
- Use minimal official images (node:20-alpine, python:3.12-slim, golang:1.22-alpine,
  mcr.microsoft.com/dotnet/sdk:8.0 for .NET, eclipse-temurin:21-jdk for Java,
  ruby:3.3-alpine, php:8.3-cli, rust:1-alpine). Match the image to the manifest
  you were given, not to the repository name.
- Separate containers for databases (postgres:16-alpine, mongo:7, redis:7-alpine)
- Next.js: node:20-alpine, expose 3000/tcp
- Hardhat/Foundry: set has_smart_contracts=true
- RAM: 512MB databases, 1024-2048MB apps
- estimated_cost_per_hour: $0.02-0.10 based on total resources`, repoURL, sb.String())

	// Call Groq via OpenAI-compatible chat completions API
	reqBody := map[string]any{
		"model":      s.model,
		"max_tokens": 4096,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	respBody, err := groq.Post(ctx, s.apiKey, bodyBytes, nil)
	if err != nil {
		return nil, fmt.Errorf("groq api: %w", err)
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("decode groq response: %w", err)
	}
	if apiResp.Error != nil {
		return nil, fmt.Errorf("groq error: %s", apiResp.Error.Message)
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("groq: empty choices")
	}

	rawJSON := strings.TrimSpace(apiResp.Choices[0].Message.Content)
	if strings.HasPrefix(rawJSON, "```") {
		lines := strings.Split(rawJSON, "\n")
		if len(lines) > 2 {
			rawJSON = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	var plan DeploymentPlan
	if err := json.Unmarshal([]byte(rawJSON), &plan); err != nil {
		return nil, fmt.Errorf("parse deployment plan JSON: %w\nraw: %s", err, rawJSON)
	}
	plan.RepoURL = repoURL
	return &plan, nil
}

// --- helpers ---

func extractRepoRoot(rawURL string) string {
	rawURL = sanitizeGitHubURL(rawURL)
	parts := strings.SplitN(rawURL, "/", 6)
	if len(parts) >= 5 && strings.Contains(rawURL, "github.com") {
		return strings.Join(parts[:5], "/")
	}
	return rawURL
}

func sanitizeGitHubURL(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, "\"'`")
	s = strings.TrimRight(s, ".,;:!?)]}>")
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	return s
}

func readTruncated(path string, maxBytes int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, maxBytes)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	content := string(buf[:n])
	if n == maxBytes {
		content += "\n... (truncated)"
	}
	return content, nil
}
