package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/shreejaykurhade/MetaMax/backend/internal/chain"
	"github.com/shreejaykurhade/MetaMax/backend/internal/container"
	"github.com/shreejaykurhade/MetaMax/backend/internal/groq"
	"github.com/shreejaykurhade/MetaMax/backend/internal/scanner"
)

// ZeroGClient is the interface for 0G Network integration.
// The agent calls these methods after each action to persist state.
type ZeroGClient interface {
	Append(ctx context.Context, logID string, entry []byte) error
	ReadLog(ctx context.Context, logID string) ([][]byte, error)
}

// Action represents a single tool call in the audit log.
type Action struct {
	Index     int            `json:"index"`
	Tool      string         `json:"tool"`
	Input     map[string]any `json:"input"`
	Result    any            `json:"result"`
	Error     string         `json:"error,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Hash      string         `json:"hash"` // SHA256(index|tool|input|result|timestamp)
}

// SessionState is the lifecycle state of an agent session.
type SessionState string

const (
	StateRunning   SessionState = "running"
	StateCompleted SessionState = "completed"
	StateFailed    SessionState = "failed"
)

// Event is streamed to the frontend over WebSocket.
type Event struct {
	Type        string  `json:"type"` // action | message | plan | stage | done | error
	Action      *Action `json:"action,omitempty"`
	Message     string  `json:"message,omitempty"`
	Plan        any     `json:"plan,omitempty"`
	ContainerID string  `json:"container_id,omitempty"`
	DeployedURL string  `json:"deployed_url,omitempty"`
	MerkleRoot  string  `json:"merkle_root,omitempty"`

	// Demo pipeline fields. Every on-chain step carries a Monad transaction
	// hash and a ready-made MonadScan link so the UI never has to build one.
	Stage       string `json:"stage,omitempty"`  // see stage constants below
	Status      string `json:"status,omitempty"` // active | done | failed | skipped
	Detail      string `json:"detail,omitempty"`
	TxHash      string `json:"tx_hash,omitempty"`
	ExplorerURL string `json:"explorer_url,omitempty"`
	Data        any    `json:"data,omitempty"`
}

// Pipeline stages for the one-button "Deploy my AI workload" demo. The agent
// emits a stage event as it enters each step and again when the step resolves.
const (
	StageAnalyzing        = "analyzing"         // AI analyzing task
	StageFindingProviders = "finding_providers" // Finding providers
	StageComparing        = "comparing"         // Comparing N nodes
	StageSelecting        = "selecting"         // Selecting Provider #N
	StageEscrow           = "escrow"            // Creating escrow
	StageEncrypting       = "encrypting"        // Encrypting workload
	StageExecuting        = "executing"         // Executing
	StageProof            = "proof"             // Generating proof
	StageAttesting        = "attesting"         // Attesting on Monad
	StageSettlement       = "settlement"        // Releasing payment
)

// PipelineStages is the ordered stage list the UI renders.
var PipelineStages = []string{
	StageAnalyzing, StageFindingProviders, StageComparing, StageSelecting,
	StageEscrow, StageEncrypting, StageExecuting, StageProof,
	StageAttesting, StageSettlement,
}

// SessionConfig carries everything a Session needs to run.
type SessionConfig struct {
	ID     string
	TeamID string

	Manager *container.Manager
	Scanner *scanner.Scanner
	ZeroG   ZeroGClient
	AXL     AXLClient // agent-to-agent transport; nil disables delegation

	GroqAPIKey string
	Model      string

	RPCURL             string
	RegistryAddress    string
	AuctionAddress     string // JobAuction contract — empty means skip the auction
	EscrowAddress      string // DeploymentEscrow contract
	AttestationAddress string // ExecutionAttestation contract
	AgentPrivKey       string // autonomous agent wallet key

	DeployDomain string
	EnvVars      map[string]string

	// Autopilot runs the whole pipeline without waiting for a human to approve
	// the plan — this is what the one-button demo uses.
	Autopilot bool

	// EscrowDepositMON is how much native MON the agent locks for the provider
	// at the escrow stage, e.g. "0.01". Empty disables agent-funded escrow.
	EscrowDepositMON string
}

// Session manages one agent deployment conversation.
type Session struct {
	ID               string
	TeamID           string
	State            SessionState
	Actions          []Action
	MerkleRoot       string
	Plan             *scanner.DeploymentPlan
	SelectedProvider *chain.Provider
	EnvVars          map[string]string // injected at session creation, passed to containers
	Autopilot        bool

	// Marketplace + chain results, surfaced by the API and the demo UI.
	ProviderRanking  []chain.ScoredProvider
	EscrowTxHash     string
	AuctionTxHash    string
	AttestTxHash     string
	SettlementTxHash string
	ReputationTxHash string

	// Finalizer runs after the Merkle root is computed but before the "done"
	// event, so on-chain attestation and settlement stream to the UI live.
	Finalizer func(ctx context.Context, s *Session)

	mgr                *container.Manager
	scanner            *scanner.Scanner
	groqAPIKey         string
	model              string
	rpcURL             string
	registryAddress    string
	auctionAddress     string
	escrowAddress      string
	attestationAddress string
	agentPrivKey       string
	escrowDepositMON   string
	agentWallet        string // derived from agentPrivKey, used in delegation messages
	deployDomain       string
	zeroG              ZeroGClient
	axl                AXLClient

	events          chan Event
	confirmCh       chan struct{}
	lastContainerID string
	lastEncrypted   bool // whether the last container's /app volume is actually LUKS-encrypted
	stageMu         sync.Mutex
	stagesSeen      map[string]bool
}

// Groq (OpenAI-compatible) API types
type groqMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded string
}

type groqRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	Messages  []groqMessage    `json:"messages"`
	Tools     []map[string]any `json:"tools,omitempty"`
}

type groqResponse struct {
	Choices []struct {
		Message      groqMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// NewSession creates a new agent session.
func NewSession(cfg SessionConfig) *Session {
	if cfg.Model == "" {
		cfg.Model = "openai/gpt-oss-120b"
	}
	if cfg.EnvVars == nil {
		cfg.EnvVars = map[string]string{}
	}
	agentWallet := ""
	if cfg.AgentPrivKey != "" {
		if addr, err := chain.AddressFromPrivateKey(cfg.AgentPrivKey); err == nil {
			agentWallet = addr.Hex()
		}
	}
	return &Session{
		ID:                 cfg.ID,
		TeamID:             cfg.TeamID,
		State:              StateRunning,
		EnvVars:            cfg.EnvVars,
		Autopilot:          cfg.Autopilot,
		mgr:                cfg.Manager,
		scanner:            cfg.Scanner,
		groqAPIKey:         cfg.GroqAPIKey,
		model:              cfg.Model,
		rpcURL:             cfg.RPCURL,
		registryAddress:    cfg.RegistryAddress,
		auctionAddress:     cfg.AuctionAddress,
		escrowAddress:      cfg.EscrowAddress,
		attestationAddress: cfg.AttestationAddress,
		agentPrivKey:       cfg.AgentPrivKey,
		escrowDepositMON:   cfg.EscrowDepositMON,
		agentWallet:        agentWallet,
		deployDomain:       cfg.DeployDomain,
		zeroG:              cfg.ZeroG,
		axl:                cfg.AXL,
		events:             make(chan Event, 256),
		confirmCh:          make(chan struct{}),
		stagesSeen:         map[string]bool{},
	}
}

// Confirm unblocks the agent if it is waiting for plan confirmation.
func (s *Session) Confirm() {
	select {
	case <-s.confirmCh:
	default:
		close(s.confirmCh)
	}
}

// Events returns the read-only event channel.
func (s *Session) Events() <-chan Event {
	return s.events
}

// Run executes the agent loop for the given user prompt.
func (s *Session) Run(ctx context.Context, userPrompt string) error {
	messages := []groqMessage{
		{Role: "system", Content: buildSystemPrompt(s.EnvVars)},
		{Role: "user", Content: userPrompt},
	}
	consecutiveNoToolTurns := 0

	s.StageEnter(StageAnalyzing, "Reading the workload prompt and planning the deployment")

	for {
		compactToolHistory(messages)
		log.Printf("[session %s] calling Groq (turn %d)...", s.ID, len(messages))
		resp, err := s.callGroq(ctx, messages)
		if err != nil {
			log.Printf("[session %s] Groq error: %v", s.ID, err)
			s.State = StateFailed
			s.emit(Event{Type: "error", Message: err.Error()})
			return err
		}
		msg := resp.Choices[0].Message
		finishReason := resp.Choices[0].FinishReason
		log.Printf("[session %s] Groq responded: finish_reason=%s tool_calls=%d", s.ID, finishReason, len(msg.ToolCalls))

		if msg.Content != "" {
			s.emit(Event{Type: "message", Message: msg.Content})
		}

		if finishReason != "tool_calls" || len(msg.ToolCalls) == 0 {
			consecutiveNoToolTurns++
			if consecutiveNoToolTurns <= 2 {
				s.emit(Event{Type: "message", Message: "Model returned text without tool calls; requesting tool-only response..."})
				messages = append(messages, groqMessage{Role: "assistant", Content: msg.Content})
				messages = append(messages, groqMessage{Role: "user", Content: "Use at least one tool call now. If a GitHub URL exists, call analyze_repo first."})
				continue
			}
			if len(s.Actions) == 0 {
				if repoURL := extractGitHubURL(userPrompt); repoURL != "" {
					s.emit(Event{Type: "message", Message: "Bootstrapping deployment planning..."})
					if err := s.bootstrapInitialPlan(ctx, repoURL); err != nil {
						s.State = StateFailed
						s.emit(Event{Type: "error", Message: err.Error()})
						return err
					}
					consecutiveNoToolTurns = 0
					messages = append(messages, groqMessage{Role: "assistant", Content: msg.Content})
					messages = append(messages, groqMessage{Role: "user", Content: "Planning is confirmed. Continue deployment by calling tools only."})
					continue
				}
			}
			break
		}
		consecutiveNoToolTurns = 0

		// Add assistant turn with tool calls to the conversation.
		messages = append(messages, groqMessage{
			Role:      "assistant",
			Content:   msg.Content,
			ToolCalls: msg.ToolCalls,
		})

		for _, tc := range msg.ToolCalls {
			log.Printf("[session %s] executing tool: %s", s.ID, tc.Function.Name)
			var toolInput map[string]any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &toolInput); err != nil {
				toolInput = map[string]any{}
			}

			result, toolErr := s.executeTool(ctx, tc.Function.Name, toolInput)
			log.Printf("[session %s] tool %s done (err=%v)", s.ID, tc.Function.Name, toolErr)

			action := Action{
				Index:     len(s.Actions),
				Tool:      tc.Function.Name,
				Input:     toolInput,
				Timestamp: time.Now().UTC(),
			}
			if toolErr != nil {
				action.Error = toolErr.Error()
			} else {
				action.Result = result
			}
			action.Hash = hashAction(action)
			s.Actions = append(s.Actions, action)
			s.emit(Event{Type: "action", Action: &action})

			// Persist to 0G Network after each action
			if s.zeroG != nil {
				if b, err := json.Marshal(action); err == nil {
					if err := s.zeroG.Append(ctx, s.ID, b); err != nil {
						log.Printf("[session %s] 0G append: %v", s.ID, err)
					}
				}
			}

			var resultContent string
			if toolErr != nil {
				resultContent = fmt.Sprintf("error: %s", toolErr.Error())
			} else {
				b, _ := json.Marshal(result)
				resultContent = string(b)
			}
			// The model's copy of the result is capped to head+tail; the full
			// result stays in the Action above for the Merkle proof and audit log.
			messages = append(messages, groqMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    truncateForModel(resultContent),
			})
		}
	}

	// Execution is over — every tool call is now a leaf in the proof.
	containerDesc := "container"
	if s.lastEncrypted {
		containerDesc = "encrypted container"
	}
	s.StageDone(StageExecuting, fmt.Sprintf("%d action(s) executed inside the %s", len(s.Actions), containerDesc), "")

	// Compute Merkle root over all action hashes
	s.StageEnter(StageProof, "Hashing every action and building the Merkle tree")
	s.MerkleRoot = computeMerkleRoot(s.Actions)
	s.State = StateCompleted
	s.StageDone(StageProof, "Merkle root 0x"+s.MerkleRoot, "")

	// Attest on Monad and settle payment before closing the stream, so the UI
	// receives both transaction links live.
	if s.Finalizer != nil {
		s.Finalizer(ctx, s)
	}

	doneEvent := Event{
		Type:       "done",
		Message:    "Deployment complete.",
		MerkleRoot: s.MerkleRoot,
		Data:       s.ChainSummary(),
	}
	if s.lastContainerID != "" {
		doneEvent.ContainerID = s.lastContainerID
		if s.deployDomain != "" {
			doneEvent.DeployedURL = fmt.Sprintf("https://%s.%s", s.lastContainerID, s.deployDomain)
		}
	}
	s.emit(doneEvent)
	return nil
}

// destroyContainer removes a container the session created.
//
// create_container registers containers as comput3-<team>-<name>, but the model
// refers back to them by the bare name it chose, which the daemon cannot
// resolve. Try the reference as given — that covers real container IDs — then
// fall back to the qualified name.
func (s *Session) destroyContainer(ctx context.Context, ref string) error {
	if ref == "" {
		return fmt.Errorf("container_id is required")
	}
	err := s.mgr.Destroy(ctx, ref)
	if err == nil || strings.HasPrefix(ref, "comput3-") || s.TeamID == "" {
		return err
	}
	if qualified := fmt.Sprintf("comput3-%s-%s", s.TeamID, ref); qualified != ref {
		if retryErr := s.mgr.Destroy(ctx, qualified); retryErr == nil {
			return nil
		}
	}
	return err
}

// executeTool dispatches named tool calls.
func (s *Session) executeTool(ctx context.Context, name string, input map[string]any) (any, error) {
	switch name {
	case "select_provider":
		s.emit(Event{Type: "message", Message: "Selecting compute provider..."})
		var (
			result any
			err    error
		)
		switch {
		case s.auctionAddress != "" && s.agentPrivKey != "":
			result, err = s.selectProviderViaAuction(ctx)
		case s.registryAddress != "":
			result, err = s.selectProviderScored(ctx)
		default:
			// No registry configured — the marketplace stages cannot run at all,
			// so mark them skipped rather than leaving the UI waiting on them.
			s.StageSkipped(StageFindingProviders, "no ProviderRegistry address configured")
			result, err = s.selectProviderFromRegistry(ctx)
			s.StageSkipped(StageComparing, "no on-chain providers to compare")
			s.StageSkipped(StageSelecting, "running on the local MetaMax node")
		}
		if err != nil {
			return nil, err
		}
		// The agent funds escrow for the provider it just chose, from its own
		// wallet, before any workload runs.
		s.fundEscrow(ctx)
		return result, nil

	case "analyze_repo":
		url := sanitizeGitHubURL(stringField(input, "github_url"))
		if url == "" {
			return nil, fmt.Errorf("github_url is required")
		}
		s.emit(Event{Type: "message", Message: fmt.Sprintf("Scanning repository: %s", url)})
		plan, err := s.scanner.AnalyzeRepo(ctx, url)
		if err != nil {
			return nil, err
		}
		s.Plan = plan
		s.StageDone(StageAnalyzing, fmt.Sprintf("Detected %d container(s): %s", len(plan.Containers), plan.Summary), "")
		return plan, nil

	case "generate_deployment_plan":
		plan := map[string]any{
			"summary":                stringField(input, "summary"),
			"estimated_cost_per_hour": float64Field(input, "estimated_cost_per_hour", 0),
			"containers":             input["containers"],
			"has_smart_contracts":    input["has_smart_contracts"],
			"status":                 "awaiting_confirmation",
		}
		s.emit(Event{Type: "plan", Plan: plan})

		// Autopilot ("Deploy my AI workload") approves its own plan: the whole
		// point of the demo is that no human is in the loop.
		if s.Autopilot {
			plan["status"] = "auto_confirmed"
			s.StageDone(StageAnalyzing, stringField(input, "summary"), "")
			s.emit(Event{Type: "message", Message: "Autopilot — plan approved by the agent, starting deployment..."})
			return plan, nil
		}

		timer := time.NewTimer(10 * time.Minute)
		defer timer.Stop()
		select {
		case <-s.confirmCh:
			plan["status"] = "confirmed"
			s.emit(Event{Type: "message", Message: "Plan confirmed — starting deployment..."})
		case <-timer.C:
			return nil, fmt.Errorf("deployment plan timed out waiting for confirmation")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return plan, nil

	case "create_container":
		s.StageEnter(StageEncrypting, "Provisioning the workload's storage volume")
		opts := container.CreateOpts{
			TeamID:    s.TeamID,
			SessionID: s.ID,
			Name:      stringField(input, "name"),
			Image:     stringField(input, "image"),
			RAMMb:     int64Field(input, "ram_mb", 2048),
			CPUCores:  float64Field(input, "cpu_cores", 1.0),
		}
		if ports, ok := input["ports"].([]any); ok {
			for _, p := range ports {
				if ps, ok := p.(string); ok {
					opts.Ports = append(opts.Ports, ps)
				}
			}
		}
		info, err := s.mgr.CreateContainer(ctx, opts)
		if err != nil {
			s.StageFailed(StageEncrypting, err.Error())
			return info, err
		}
		if info != nil {
			s.mgr.RegisterDeploy(info.ID, info.Ports)
			s.lastContainerID = info.ID
			s.lastEncrypted = info.Encrypted
			if info.Encrypted {
				s.StageDone(StageEncrypting, fmt.Sprintf("Encrypted container %s ready — LUKS2 key never leaves the vault", shortID(info.ID)), "")
				s.StageEnter(StageExecuting, "Running the workload inside the encrypted container")
			} else {
				// LUKS fell back to a plain volume — say so rather than claim
				// encryption the workload does not have.
				s.StageSkipped(StageEncrypting, "volume encryption unavailable on this host — running with container isolation only")
				s.StageEnter(StageExecuting, "Running the workload inside the container")
			}
		}
		return info, nil

	case "install_packages":
		id := stringField(input, "container_id")
		mgr := container.PackageManager(stringField(input, "manager"))
		var pkgs []string
		if raw, ok := input["packages"].([]any); ok {
			for _, p := range raw {
				if ps, ok := p.(string); ok {
					pkgs = append(pkgs, ps)
				}
			}
		}
		return nil, s.mgr.InstallPackages(ctx, id, pkgs, mgr)

	case "configure_network":
		var ids []string
		if raw, ok := input["container_ids"].([]any); ok {
			for _, p := range raw {
				if ps, ok := p.(string); ok {
					ids = append(ids, ps)
				}
			}
		}
		if err := s.mgr.CreateNetwork(ctx, s.TeamID); err != nil {
			return nil, err
		}
		return nil, s.mgr.ConnectContainers(ctx, s.TeamID, ids)

	case "setup_ide":
		return nil, s.mgr.SetupIDE(ctx, stringField(input, "container_id"),
			container.IDEType(stringField(input, "type")))

	case "setup_database":
		return s.mgr.SetupDatabase(ctx, s.TeamID, s.ID,
			container.DBType(stringField(input, "type")),
			stringField(input, "version"))

	case "health_check":
		return s.mgr.HealthCheck(ctx, stringField(input, "container_id"))

	case "get_logs":
		logs, err := s.mgr.GetLogs(ctx, stringField(input, "container_id"),
			int(int64Field(input, "lines", 50)))
		return map[string]string{"logs": logs}, err

	case "destroy_container":
		return nil, s.destroyContainer(ctx, stringField(input, "container_id"))

	case "clone_repo":
		id := stringField(input, "container_id")
		url := sanitizeGitHubURL(stringField(input, "github_url"))
		if url == "" {
			return nil, fmt.Errorf("github_url is required")
		}
		s.emit(Event{Type: "message", Message: fmt.Sprintf("Cloning %s into %s...", url, id)})
		out, err := s.mgr.CloneRepo(ctx, id, url, stringField(input, "directory"))
		return map[string]string{"output": out}, err

	case "run_command":
		id := stringField(input, "container_id")
		workDir := stringField(input, "work_dir")
		if workDir == "" {
			workDir = "/app"
		}
		s.emit(Event{Type: "message", Message: fmt.Sprintf("Running: %s", stringField(input, "command"))})
		out, err := s.mgr.RunCommand(ctx, id, stringField(input, "command"), workDir, mapField(input, "env"))
		return map[string]string{"output": out}, err

	case "start_process":
		id := stringField(input, "container_id")
		workDir := stringField(input, "work_dir")
		if workDir == "" {
			workDir = "/app"
		}
		s.emit(Event{Type: "message", Message: fmt.Sprintf("Starting process: %s", stringField(input, "command"))})
		out, err := s.mgr.StartProcess(ctx, id, stringField(input, "command"), workDir, mapField(input, "env"))
		return map[string]string{"output": out}, err

	case "delegate_compute":
		command := stringField(input, "command")
		if command == "" {
			return nil, fmt.Errorf("command is required")
		}
		return s.delegateCompute(ctx, command, stringField(input, "image"))

	case "write_file":
		path := stringField(input, "path")
		s.emit(Event{Type: "message", Message: fmt.Sprintf("Writing %s", path)})
		return map[string]string{"path": path},
			s.mgr.WriteFile(ctx, stringField(input, "container_id"), path, stringField(input, "content"))

	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// toolsForState returns the tool subset the model should see this turn. Before a
// container exists the agent is still planning, so it only needs the planning
// tools; once a container is up it needs the execution tools but never the
// planning ones again. Sending the smaller set on every turn is the biggest
// single per-request saving, since the tool block is re-sent on every turn.
func (s *Session) toolsForState() []map[string]any {
	if s.lastContainerID == "" {
		return pickTools(planningTools...)
	}
	return executionTools()
}

// callGroq sends the conversation to the Groq chat completions API (OpenAI-compatible).
func (s *Session) callGroq(ctx context.Context, messages []groqMessage) (*groqResponse, error) {
	tools := s.toolsForState()
	req := groqRequest{
		Model:     s.model,
		MaxTokens: 8192,
		Messages:  messages,
		Tools:     tools,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	logRequestSize(s.ID, body, messages, tools)

	respBody, err := groq.Post(ctx, s.groqAPIKey, body, func(wait time.Duration, attempt int, reason string) {
		log.Printf("[session %s] groq %s, retrying in %.1fs (attempt %d)", s.ID, reason, wait.Seconds(), attempt)
		s.emit(Event{
			Type:    "message",
			Message: fmt.Sprintf("Model %s — retrying in %.0fs...", reason, wait.Seconds()),
		})
	})
	if err != nil {
		return nil, err
	}

	var resp groqResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse groq response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("groq error: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("groq: empty choices")
	}
	return &resp, nil
}

// --- Context-size management ---
//
// Two forces grow a request over a deploy: the tool block (handled by phase
// gating in toolsForState) and the conversation history. History growth is
// dominated by tool results — a single build log (npm ci, compilation) can run
// to tens of kilobytes and, left whole, is re-sent on every later turn. These
// helpers bound that: each result is capped to head+tail as it enters history,
// and all but the most recent few are collapsed to a stub before each call.

const (
	// maxToolResultChars is the largest tool result fed back to the model. Above
	// it, only the head and tail are kept — enough to see the command that ran
	// and its final status — and the middle is dropped.
	maxToolResultChars = 2000
	toolResultHead     = 1200
	toolResultTail     = 600

	// keepVerbatimResults is how many of the most recent tool results stay full
	// in the conversation. Older results are stubbed: the assistant's own tool
	// call (name + arguments) remains in history, so the model still knows what
	// ran — it just no longer needs the verbose output many turns later.
	keepVerbatimResults = 6

	toolResultStub = "[earlier tool result omitted to conserve context]"
)

// truncateForModel shrinks an oversized tool result to its head and tail, noting
// how many bytes were elided.
func truncateForModel(s string) string {
	if len(s) <= maxToolResultChars {
		return s
	}
	elided := len(s) - toolResultHead - toolResultTail
	return fmt.Sprintf("%s\n...[%d bytes elided]...\n%s", s[:toolResultHead], elided, s[len(s)-toolResultTail:])
}

// compactToolHistory collapses every tool result except the most recent
// keepVerbatimResults into a stub, in place. This bounds how large the
// conversation can grow over a long deploy: without it every past result is
// re-sent on every remaining turn. It is idempotent — an already-stubbed result
// is left alone — so it is safe to call before every request.
func compactToolHistory(messages []groqMessage) {
	seen := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "tool" {
			continue
		}
		seen++
		if seen <= keepVerbatimResults {
			continue
		}
		if messages[i].Content != toolResultStub {
			messages[i].Content = toolResultStub
		}
	}
}

// estTokens is a rough proxy for the billed token count — Groq uses real BPE
// tokens, but bytes/4 tracks the trend closely enough to watch the effect of
// trimming from turn to turn in the logs.
func estTokens(n int) int { return n / 4 }

// logRequestSize records the size of each outbound request so the token cost of
// a deploy can be measured directly: total request bytes, the estimated token
// count, and how much of that is the (phase-gated) tool block versus the full
// catalogue it was trimmed from.
func logRequestSize(id string, body []byte, messages []groqMessage, tools []map[string]any) {
	sentTools, _ := json.Marshal(tools)
	fullTools, _ := json.Marshal(toolDefinitions)
	log.Printf("[session %s] groq request: %d msgs, %d bytes (~%d tok) | tools %d/%d sent, %d/%d bytes",
		id, len(messages), len(body), estTokens(len(body)),
		len(tools), len(toolDefinitions), len(sentTools), len(fullTools))
}

// selectProviderFromRegistry reads the ProviderRegistry directly, picks the cheapest
// active provider, and health-checks its endpoint before returning.
func (s *Session) selectProviderFromRegistry(ctx context.Context) (any, error) {
	s.emit(Event{Type: "message", Message: "Querying ProviderRegistry on Monad Testnet..."})
	provider, err := chain.SelectCheapestProvider(ctx, s.rpcURL, s.registryAddress)
	if err != nil {
		s.emit(Event{Type: "message", Message: fmt.Sprintf("Warning: chain query failed (%v). Using local node.", err)})
		return map[string]any{"endpoint": "http://localhost:8081", "price_per_hour": "0", "source": "fallback"}, nil
	}
	// Health-check the provider before accepting.
	healthURL := strings.TrimRight(provider.Endpoint, "/") + "/health"
	pingCtx, pingCancel := context.WithTimeout(ctx, 3*time.Second)
	defer pingCancel()
	if req, perr := http.NewRequestWithContext(pingCtx, http.MethodGet, healthURL, nil); perr == nil {
		if resp, perr := http.DefaultClient.Do(req); perr != nil || resp.StatusCode >= 500 {
			s.emit(Event{Type: "message", Message: fmt.Sprintf("Warning: provider health check failed (%v). Using local node.", perr)})
			return map[string]any{"endpoint": "http://localhost:8081", "price_per_hour": "0", "source": "fallback"}, nil
		}
	}
	s.SelectedProvider = provider
	return map[string]any{
		"wallet":         provider.Wallet.Hex(),
		"endpoint":       provider.Endpoint,
		"price_per_hour": provider.PricePerHour.String(),
		"jobs_completed": provider.JobsCompleted.Uint64(),
		"source":         "on-chain",
	}, nil
}

// selectProviderViaAuction posts a job to the JobAuction contract, waits 30 s for
// provider bids, closes the auction, and returns the winning provider.
// Falls back to selectProviderFromRegistry on any error.
func (s *Session) selectProviderViaAuction(ctx context.Context) (any, error) {
	jobID := chain.SessionIDToJobID(s.ID)

	// Derive resource requirements from the deployment plan.
	ramMb := big.NewInt(2048)
	cpuCores := big.NewInt(1)
	if s.Plan != nil && len(s.Plan.Containers) > 0 {
		var totalRAM int64
		for _, c := range s.Plan.Containers {
			totalRAM += c.RAMMb
			if cores := int64(c.CPUCores); cores > cpuCores.Int64() {
				cpuCores = big.NewInt(cores)
			}
		}
		if totalRAM > 0 {
			ramMb = big.NewInt(totalRAM)
		}
		if cpuCores.Sign() == 0 {
			cpuCores = big.NewInt(1)
		}
	}

	// Ceiling price: 0.01 ETH/hr. Deposit: same (covers exactly 1 hr).
	maxPricePerHour := big.NewInt(10_000_000_000_000_000) // 0.01 ETH/hr
	durationSeconds := big.NewInt(3600)
	depositWei := big.NewInt(10_000_000_000_000_000) // 0.01 ETH

	s.StageEnter(StageFindingProviders, "Posting the job to JobAuction (30 s bid window)")
	postTx, err := chain.PostJob(ctx, s.rpcURL, s.agentPrivKey, s.auctionAddress,
		jobID, maxPricePerHour, ramMb, cpuCores, durationSeconds, depositWei)
	if err != nil {
		s.StageFailed(StageFindingProviders, fmt.Sprintf("auction post failed: %v — falling back to the registry", err))
		return s.selectProviderScored(ctx)
	}
	s.AuctionTxHash = postTx
	s.StageDone(StageFindingProviders, "Job posted on-chain — providers can bid", postTx)

	s.StageEnter(StageComparing, "Collecting sealed provider bids for 30 seconds")

	// Wait for bid window, then close the auction.
	watchCtx, watchCancel := context.WithTimeout(ctx, 55*time.Second)
	defer watchCancel()

	// Fire closeAuction ~1 s after the 30 s window expires.
	go func() {
		select {
		case <-time.After(31 * time.Second):
			if _, cerr := chain.CloseAuction(watchCtx, s.rpcURL, s.agentPrivKey, s.auctionAddress, jobID); cerr != nil {
				log.Printf("[session %s] CloseAuction: %v", s.ID, cerr)
			} else {
				log.Printf("[session %s] CloseAuction submitted", s.ID)
			}
		case <-watchCtx.Done():
		}
	}()

	s.emit(Event{Type: "message", Message: "Waiting for auction result..."})
	awarded, err := chain.WatchJobAwarded(watchCtx, s.rpcURL, s.auctionAddress, jobID)
	if err != nil {
		s.StageFailed(StageComparing, fmt.Sprintf("no auction result: %v — falling back to the registry", err))
		return s.selectProviderScored(ctx)
	}

	source := "auction-winner"
	if awarded.IsFallback {
		source = "auction-fallback"
	}
	s.StageDone(StageComparing, fmt.Sprintf("Auction closed — lowest valid bid %s wei/hr", awarded.PricePerHour), "")
	s.emit(Event{
		Type:        "stage",
		Stage:       StageSelecting,
		Status:      "done",
		Detail:      fmt.Sprintf("Auction winner %s at %s wei/hr (%s)", shortAddr(awarded.Winner.Hex()), awarded.PricePerHour, source),
		ExplorerURL: chain.MonadAddressURL(awarded.Winner.Hex()),
	})

	// Look up the winner's endpoint from the registry.
	providers, err := chain.GetActiveProviders(ctx, s.rpcURL, s.registryAddress)
	if err == nil {
		for i := range providers {
			if providers[i].Wallet == awarded.Winner {
				s.SelectedProvider = &providers[i]
				return map[string]any{
					"wallet":          providers[i].Wallet.Hex(),
					"endpoint":        providers[i].Endpoint,
					"price_per_hour":  awarded.PricePerHour.String(),
					"rate_per_second": awarded.RatePerSecond.String(),
					"jobs_completed":  providers[i].JobsCompleted.Uint64(),
					"source":          source,
				}, nil
			}
		}
	}

	// Winner not in current registry view — construct minimal provider from event data.
	s.SelectedProvider = &chain.Provider{
		Wallet:       awarded.Winner,
		PricePerHour: awarded.PricePerHour,
		Active:       true,
		Endpoint:     "http://localhost:8081",
	}
	return map[string]any{
		"wallet":          awarded.Winner.Hex(),
		"price_per_hour":  awarded.PricePerHour.String(),
		"rate_per_second": awarded.RatePerSecond.String(),
		"source":          source,
	}, nil
}

func (s *Session) bootstrapInitialPlan(ctx context.Context, repoURL string) error {
	analyzeResult, err := s.executeToolAndRecord(ctx, "analyze_repo", map[string]any{"github_url": repoURL})
	if err != nil {
		return err
	}
	_, _ = s.executeToolAndRecord(ctx, "select_provider", map[string]any{})

	plan, ok := analyzeResult.(*scanner.DeploymentPlan)
	if !ok || plan == nil {
		return fmt.Errorf("bootstrap analyze_repo returned invalid plan")
	}
	_, err = s.executeToolAndRecord(ctx, "generate_deployment_plan", map[string]any{
		"summary":                 plan.Summary,
		"estimated_cost_per_hour": plan.EstimatedCostPerHour,
		"containers":              plan.Containers,
		"has_smart_contracts":     plan.HasSmartContracts,
	})
	return err
}

func (s *Session) executeToolAndRecord(ctx context.Context, name string, input map[string]any) (any, error) {
	result, toolErr := s.executeTool(ctx, name, input)
	action := Action{
		Index:     len(s.Actions),
		Tool:      name,
		Input:     input,
		Timestamp: time.Now().UTC(),
	}
	if toolErr != nil {
		action.Error = toolErr.Error()
	} else {
		action.Result = result
	}
	action.Hash = hashAction(action)
	s.Actions = append(s.Actions, action)
	s.emit(Event{Type: "action", Action: &action})
	if s.zeroG != nil {
		if b, err := json.Marshal(action); err == nil {
			if err := s.zeroG.Append(ctx, s.ID, b); err != nil {
				log.Printf("[session %s] 0G append: %v", s.ID, err)
			}
		}
	}
	return result, toolErr
}

func (s *Session) emit(e Event) {
	select {
	case s.events <- e:
	default:
	}
}

// --- Merkle root computation ---

// hashAction computes SHA256(index|tool|JSON(input)|JSON(result)|ISO8601(timestamp)).
func hashAction(a Action) string {
	resultBytes, _ := json.Marshal(a.Result)
	inputBytes, _ := json.Marshal(a.Input)
	preimage := fmt.Sprintf("%d|%s|%s|%s|%s",
		a.Index, a.Tool, string(inputBytes), string(resultBytes), a.Timestamp.UTC().Format(time.RFC3339))
	sum := sha256.Sum256([]byte(preimage))
	return fmt.Sprintf("%x", sum)
}

// computeMerkleRoot builds a binary Merkle tree over action hashes and returns the root.
func computeMerkleRoot(actions []Action) string {
	if len(actions) == 0 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte{}))
	}
	leaves := make([][32]byte, len(actions))
	for i, a := range actions {
		h, _ := hexToHash(a.Hash)
		leaves[i] = h
	}
	root := merkleRoot(leaves)
	return fmt.Sprintf("%x", root)
}

func merkleRoot(nodes [][32]byte) [32]byte {
	if len(nodes) == 1 {
		return nodes[0]
	}
	if len(nodes)%2 != 0 {
		nodes = append(nodes, nodes[len(nodes)-1]) // duplicate last leaf
	}
	var next [][32]byte
	for i := 0; i < len(nodes); i += 2 {
		combined := append(nodes[i][:], nodes[i+1][:]...)
		next = append(next, sha256.Sum256(combined))
	}
	return merkleRoot(next)
}

// ComputeMerkleProof returns the sibling hashes from leaf to root for the action at leafIndex.
// Each entry is prefixed "left:<hex>" or "right:<hex>" indicating which side the sibling sits on.
func ComputeMerkleProof(actions []Action, leafIndex int) []string {
	if len(actions) == 0 || leafIndex < 0 || leafIndex >= len(actions) {
		return nil
	}
	leaves := make([][32]byte, len(actions))
	for i, a := range actions {
		h, _ := hexToHash(a.Hash)
		leaves[i] = h
	}
	return merkleProof(leaves, leafIndex)
}

func merkleProof(nodes [][32]byte, index int) []string {
	if len(nodes) <= 1 {
		return nil
	}
	if len(nodes)%2 != 0 {
		nodes = append(nodes, nodes[len(nodes)-1])
	}
	var label string
	var sibling [32]byte
	if index%2 == 0 {
		label = "right"
		sibling = nodes[index+1]
	} else {
		label = "left"
		sibling = nodes[index-1]
	}
	proof := []string{fmt.Sprintf("%s:%x", label, sibling)}
	var next [][32]byte
	for i := 0; i < len(nodes); i += 2 {
		combined := append(nodes[i][:], nodes[i+1][:]...)
		next = append(next, sha256.Sum256(combined))
	}
	return append(proof, merkleProof(next, index/2)...)
}

func hexToHash(h string) ([32]byte, error) {
	var out [32]byte
	h = strings.TrimPrefix(h, "sha256:")
	if len(h) < 64 {
		return out, fmt.Errorf("short hash")
	}
	for i := 0; i < 32; i++ {
		b := 0
		fmt.Sscanf(h[i*2:i*2+2], "%02x", &b)
		out[i] = byte(b)
	}
	return out, nil
}

// --- input helpers ---

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func int64Field(m map[string]any, key string, def int64) int64 {
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	}
	return def
}

func float64Field(m map[string]any, key string, def float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return def
}

func mapField(m map[string]any, key string) map[string]string {
	out := make(map[string]string)
	raw, ok := m[key].(map[string]any)
	if !ok {
		return out
	}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func extractGitHubURL(input string) string {
	re := regexp.MustCompile(`https://github\.com/[\w\-.]+/[\w\-.]+`)
	match := re.FindString(input)
	return sanitizeGitHubURL(match)
}

func sanitizeGitHubURL(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, "\"'`")
	s = strings.TrimRight(s, ".,;:!?)]}>")
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	return s
}
