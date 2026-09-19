package agent

import "strings"

// toolDefinitions are the tools available to the Groq model (OpenAI-compatible format).
var toolDefinitions = []map[string]any{
	{
		"type": "function",
		"function": map[string]any{
			"name":        "analyze_repo",
			"description": "Analyze a GitHub repository to detect the tech stack and generate a deployment plan. Always call this first when given a repo URL.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"github_url": map[string]any{"type": "string", "description": "Full GitHub repository URL"},
				},
				"required": []string{"github_url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "select_provider",
			"description": "Query the ProviderRegistry on Monad Testnet to find the cheapest active compute provider. Call this after analyze_repo.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "generate_deployment_plan",
			"description": "Present the deployment plan to the user and wait for confirmation before provisioning anything.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"summary":                 map[string]any{"type": "string", "description": "Human-readable summary of what will be deployed"},
					"estimated_cost_per_hour": map[string]any{"type": "number", "description": "Estimated cost in USD per hour"},
					"containers":              map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
					"has_smart_contracts":     map[string]any{"type": "boolean"},
				},
				"required": []string{"summary", "estimated_cost_per_hour"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "create_container",
			"description": "Create and start a Docker container for the team.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":      map[string]any{"type": "string"},
					"image":     map[string]any{"type": "string"},
					"ram_mb":    map[string]any{"type": "integer"},
					"cpu_cores": map[string]any{"type": "number"},
					"ports":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"name", "image"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "install_packages",
			"description": "Install packages inside a running container.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"container_id": map[string]any{"type": "string"},
					"packages":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"manager":      map[string]any{"type": "string", "enum": []string{"npm", "pip", "apt"}},
				},
				"required": []string{"container_id", "packages", "manager"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "configure_network",
			"description": "Connect containers to a shared team network.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"container_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"container_ids"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "setup_ide",
			"description": "Install a browser IDE (VS Code or Jupyter) in a container.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"container_id": map[string]any{"type": "string"},
					"type":         map[string]any{"type": "string", "enum": []string{"vscode", "jupyter"}},
				},
				"required": []string{"container_id", "type"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "setup_database",
			"description": "Start a managed database container.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":    map[string]any{"type": "string", "enum": []string{"postgres", "mongo", "redis", "mysql"}},
					"version": map[string]any{"type": "string"},
				},
				"required": []string{"type"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "health_check",
			"description": "Check if a container is running.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"container_id": map[string]any{"type": "string"},
				},
				"required": []string{"container_id"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "get_logs",
			"description": "Retrieve recent log output from a container.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"container_id": map[string]any{"type": "string"},
					"lines":        map[string]any{"type": "integer"},
				},
				"required": []string{"container_id"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "destroy_container",
			"description": "Stop and remove a container.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"container_id": map[string]any{"type": "string"},
				},
				"required": []string{"container_id"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "clone_repo",
			"description": "Clone a GitHub repository into a container's /app directory.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"container_id": map[string]any{"type": "string"},
					"github_url":   map[string]any{"type": "string"},
					"directory":    map[string]any{"type": "string"},
				},
				"required": []string{"container_id", "github_url"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "run_command",
			"description": "Run a shell command inside a container and return its output. Use for build steps: npm install, pip install, go build, etc.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"container_id": map[string]any{"type": "string"},
					"command":      map[string]any{"type": "string"},
					"work_dir":     map[string]any{"type": "string"},
					"env":          map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
				},
				"required": []string{"container_id", "command"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "start_process",
			"description": "Start a long-running application process in the background.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"container_id": map[string]any{"type": "string"},
					"command":      map[string]any{"type": "string"},
					"work_dir":     map[string]any{"type": "string"},
					"env":          map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
				},
				"required": []string{"container_id", "command"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name":        "write_file",
			"description": "Write a text file into a container (for .env, config files, etc.).",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"container_id": map[string]any{"type": "string"},
					"path":         map[string]any{"type": "string"},
					"content":      map[string]any{"type": "string"},
				},
				"required": []string{"container_id", "path", "content"},
			},
		},
	},
	{
		"type": "function",
		"function": map[string]any{
			"name": "delegate_compute",
			"description": "Hand a self-contained subtask to a peer MetaMax agent over the Gensyn AXL network and wait for its result. " +
				"Use this only for work that can run independently of this session's container, such as a heavy build or a model evaluation. " +
				"If no peer answers, run the work locally instead.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string", "description": "Shell command the peer agent should run"},
					"image":   map[string]any{"type": "string", "description": "Optional container image the peer should use"},
				},
				"required": []string{"command"},
			},
		},
	},
}

const baseSystemPrompt = `You are a deployment agent for MetaMax, a decentralized trustless compute platform for AI agents.

When a user provides a GitHub URL, follow this EXACT sequence:
1. Call analyze_repo(github_url) — scans the repo and returns a deployment plan
2. Call select_provider() — picks the cheapest active provider from the on-chain registry
3. Call generate_deployment_plan(...) — presents the plan to the user; blocks until confirmed
4. WAIT for user confirmation (generate_deployment_plan blocks until the user calls POST /sessions/:id/confirm)
5. Call create_container() for each container in the plan
6. Call clone_repo() to clone the repository into each container
7. Call run_command() to install dependencies
8. If env vars are provided below, write them to /app/.env inside the container with write_file
9. Call start_process() to start the application
10. Call health_check() to verify each container is running
11. Done — the deployment is complete.

Rules:
- Never skip generate_deployment_plan. The user MUST confirm before any container is created.
- Use only official minimal Docker images.
- Always call health_check after start_process.
- If a step fails, report the error via get_logs and stop.
- Only use the tools provided — never ask for raw shell access.
- If env vars are provided, ALWAYS write them to /app/.env before starting the process.`

// buildSystemPrompt returns the system prompt, optionally appending env var injection instructions.
func buildSystemPrompt(envVars map[string]string) string {
	if len(envVars) == 0 {
		return baseSystemPrompt
	}

	var sb strings.Builder
	sb.WriteString(baseSystemPrompt)
	sb.WriteString("\n\n## Environment Variables\n\nInject the following environment variables into ALL containers.")
	sb.WriteString("\nWrite them to /app/.env using write_file AND pass them as environment when calling start_process:\n\n```\n")
	for k, v := range envVars {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
		sb.WriteString("\n")
	}
	sb.WriteString("```\n")
	return sb.String()
}
