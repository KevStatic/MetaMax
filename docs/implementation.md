# MetaMax — Implementation Plan

This document defines every module in the system, its responsibilities, inputs/outputs, and dependencies. Use this as the engineering spec during the hackathon.

---

## Module Map

```
backend/
├── cmd/server/main.go          Entry point — wires all modules
├── internal/
│   ├── agent/                  AI agent loop (Claude tool use)
│   ├── api/                    HTTP/WebSocket server
│   ├── chain/                  On-chain reads/writes (raw JSON-RPC)
│   ├── container/              Docker + LUKS management
│   ├── scanner/                Repo/task analysis via LLM
│   ├── store/                  PostgreSQL persistence
│   └── config/                 Environment config loading
integrations/
├── 0g/                         0G Network KV/log client
├── axl/                        Gensyn AXL pub/sub client
└── keeperhub/                  KeeperHub job registration
contracts/
├── ProviderRegistry.sol        Provider staking and selection
├── DeploymentEscrow.sol        MON escrow + streaming payment
└── JobAuction.sol              Bid-based provider matching
```

---

## Module 1 — Agent (`backend/internal/agent/`)

### Responsibilities
- Owns the full lifecycle of one execution session
- Translates user intent into structured tool calls
- Sequences tool calls in the correct order
- Emits events to the frontend over WebSocket
- Builds the action log for Merkle verification

### Inputs
- User prompt (string)
- GitHub URL or task description
- Confirmed `DeploymentPlan` (from scanner)
- Selected `Provider` (from chain)

### Outputs
- Stream of `Event` structs: `plan`, `action`, `message`, `done`, `error`
- Completed `[]Action` slice (each with SHA256 hash)
- Final `merkleRoot` (hex string)

### Tools available to Claude
| Tool | Action |
|---|---|
| `analyze_repo` | Calls `scanner.Scan()`, returns `DeploymentPlan` |
| `select_provider` | Calls `chain.GetActiveProviders()`, picks cheapest |
| `generate_deployment_plan` | Presents plan, blocks on `confirmCh` |
| `create_container` | Calls `container.Manager.Create()` |
| `install_packages` | Calls `container.Manager.Exec()` with package manager |
| `configure_network` | Connects containers to shared team network |
| `exec_command` | Runs arbitrary command inside container |
| `deploy_workspace` | Provisions VS Code Server or Jupyter |

### Dependencies
- `container.Manager`
- `scanner.Scanner`
- `chain` package (provider selection)
- Anthropic Claude API

### Key data structures
```go
type Action struct {
    Index     int
    Tool      string
    Input     map[string]any
    Result    any
    Error     string
    Timestamp time.Time
    Hash      string  // SHA256(index|tool|input|result|timestamp)
}

type Session struct {
    ID        string
    TeamID    string
    State     SessionState   // running | completed | failed
    Actions   []Action
    Plan      *DeploymentPlan
    Provider  *chain.Provider
    events    chan Event
    confirmCh chan struct{}
}
```

### 0G Integration point
After each action, append to 0G session log:
```go
zeroG.Append(sessionID, action)
```
On session start, load prior state from 0G to enable resumability.

---

## Module 2 — API Layer (`backend/internal/api/`)

### Responsibilities
- HTTP routing and middleware
- x402 payment verification before compute endpoints
- JWT issuance and validation (SIWE-based wallet auth)
- WebSocket session streaming
- Subdomain proxy for deployed workspaces

### Endpoints

| Method | Path | Description |
|---|---|---|
| POST | `/auth/nonce` | Issue sign challenge for wallet |
| POST | `/auth/verify` | Verify SIWE signature, return JWT |
| POST | `/sessions` | Start new execution session (x402 gated) |
| GET | `/sessions/{id}` | Get session state |
| POST | `/sessions/{id}/confirm` | User confirms plan |
| GET | `/sessions/{id}/stream` | WebSocket event stream |
| GET | `/sessions/{id}/audit` | Full action log with hashes |
| GET | `/providers/active` | List active providers from chain |
| GET | `/teams/{id}/workspaces` | List provisioned workspaces |
| GET | `/vault/nonce` | Vault key request challenge |
| POST | `/vault/key` | Return blockchain-gated decryption key |
| GET | `/health` | Liveness probe |

### x402 Flow
1. Client calls POST `/sessions` without payment → 402 with `X-Payment-Required` body
2. Client signs USDC `transferWithAuthorization` with wallet
3. Client retries with `X-Payment: <base64-encoded-sig>` header
4. Middleware verifies signature on-chain, records payment, allows request

### Dependencies
- `agent.Session`
- `container.Manager`
- `chain` package
- `store.Store`
- `auth.Manager`

### KeeperHub Integration point
After session completion, register EAS attestation as a KeeperHub job:
```go
keeperhub.RegisterJob("submit-attestation", sessionID, attestationPayload)
```

---

## Module 3 — Container Manager (`backend/internal/container/`)

### Responsibilities
- Create and destroy Docker containers with resource limits
- Set up and tear down LUKS2 encrypted volumes
- Execute commands inside containers
- Track workspace → port mappings
- Provide SSH access gateway

### Inputs
```go
type CreateOpts struct {
    TeamID   string
    Name     string
    Image    string
    RAMMb    int64
    CPUCores float64
    Ports    []string
    VaultKey string  // hex AES-256; derived from HMAC(masterSecret, containerID)
}
```

### Outputs
```go
type ContainerInfo struct {
    ID     string
    Name   string
    Status string
    Ports  map[string]string  // "3000/tcp" → "32100"
}
```

### LUKS Lifecycle
```
Create:
  1. dd /dev/urandom → vault.img (512MB)
  2. losetup → attach loop device
  3. cryptsetup luksFormat → format with vault key
  4. cryptsetup open → map to /dev/mapper/zkloud-{suffix}
  5. mkfs.ext4 → format plaintext filesystem
  6. mount → /container-storage/{id}/home
  7. bind-mount /home into container at /app

Destroy:
  1. umount /container-storage/{id}/home
  2. cryptsetup close mapper
  3. losetup -d loop device
  4. rm vault.img (optional; keep for persistence)
```

### Dependencies
- Docker SDK (`moby/moby/client`)
- `cryptsetup` (host binary)
- `losetup` (host binary)

---

## Module 4 — Scanner (`backend/internal/scanner/`)

### Responsibilities
- Given a GitHub URL or task description, produce a structured `DeploymentPlan`
- Detect tech stack, required images, ports, dependencies
- Estimate resource requirements and cost

### Inputs
- GitHub repository URL (string)
- Optional: task freeform description

### Outputs
```go
type DeploymentPlan struct {
    Summary         string
    TechStack       []string
    Containers      []ContainerSpec
    HasContracts    bool
    EstimatedCostUSD float64  // per hour
}

type ContainerSpec struct {
    Name    string
    Image   string
    RAMMb   int64
    CPU     float64
    Ports   []string
    EnvVars map[string]string
}
```

### Dependencies
- LLM API (Ollama or Claude) for repo analysis
- `go-git` for shallow clone + file inspection

---

## Module 5 — Chain (`backend/internal/chain/`)

### Responsibilities
- Read active providers from `ProviderRegistry`
- Submit EAS attestations
- Verify USDC `transferWithAuthorization` signatures
- Derive and gate vault keys using on-chain data

### Key functions
| Function | Description |
|---|---|
| `GetActiveProviders(ctx, rpcURL, registryAddr)` | Returns `[]Provider` sorted by price |
| `SubmitAttestation(ctx, rpcURL, key, schemaUID, data)` | Sends EAS.attest() tx |
| `VerifyUSDCTransfer(ctx, rpcURL, sig, data)` | Verifies x402 payment signature |
| `DeriveVaultKey(masterSecret, containerID)` | HMAC-SHA256 key derivation |

### Design note
All on-chain reads use raw JSON-RPC (`eth_call`) rather than `go-ethereum/ethclient`. This keeps the binary smaller and avoids the overhead of full ABI binding for simple read operations.

### Dependencies
- Monad Testnet RPC endpoint
- `go-ethereum` (ABI encoding + crypto primitives only)

---

## Module 6 — Store (`backend/internal/store/`)

### Responsibilities
- Persist sessions, teams, action logs, payments
- Provide queryable audit trail per session
- Track x402 payment records to prevent replay

### Schema (simplified)

```sql
teams       (id, wallet_address, name, created_at)
sessions    (id, team_id, prompt, state, merkle_root, attestation_tx, created_at, updated_at)
action_logs (id, session_id, action_index, tool, input_json, result_json, hash, timestamp)
payments    (id, wallet, nonce, amount_usdc, session_id, verified_at)
containers  (id, team_id, session_id, docker_id, name, image, status, ports_json, created_at)
```

### Dependencies
- PostgreSQL
- `jackc/pgx/v5`

---

## Module 7 — Verification (inline in agent + chain)

### Responsibilities
- Hash each action at execution time
- Compute Merkle root over all action hashes at session end
- Submit root via EAS attestation

### Hash computation
```
actionHash[i] = SHA256(
    strconv.Itoa(i) +
    action.Tool +
    string(json.Marshal(action.Input)) +
    string(json.Marshal(action.Result)) +
    action.Timestamp.Format(time.RFC3339Nano)
)
```

### Merkle construction
```
Layer 0: [h0, h1, h2, h3, ...]           (action hashes)
Layer 1: [SHA256(h0+h1), SHA256(h2+h3)]  (pairs)
...
Root:    single hash
```
Odd nodes are hashed with themselves (standard Bitcoin-style padding).

### EAS attestation schema
```
schemaUID: <registered on Monad Testnet>
data: ABI-encode(sessionID bytes32, teamID address, merkleRoot bytes32, providerAddress address, actionCount uint32)
recipient: provider wallet
```

---

## Integration 1 — 0G Network (`integrations/0g/`)

### Purpose
Decentralized key-value store for agent memory and execution logs. Allows sessions to survive node restarts and enables cross-node agent state sharing.

### Interface
```go
type Client interface {
    Put(ctx context.Context, key string, value []byte) error
    Get(ctx context.Context, key string) ([]byte, error)
    Append(ctx context.Context, logID string, entry []byte) error
    ReadLog(ctx context.Context, logID string) ([][]byte, error)
}
```

### Usage pattern
```
key: "session:{sessionID}:state"     → serialized Session struct
key: "session:{sessionID}:log"       → append-only action log
key: "agent:{teamID}:memory"         → persistent agent KV for cross-session context
```

### Configuration
```
0G_RPC_URL=
0G_PRIVATE_KEY=     (funded wallet for storage fees)
0G_FLOW_ADDRESS=    (0G Flow contract address)
```

---

## Integration 2 — Gensyn AXL (`integrations/axl/`)

### Purpose
Pub/sub messaging protocol for agent-to-agent communication across provider nodes. Used in multi-agent workflows where a parent agent delegates subtasks to specialists on other nodes.

### Interface
```go
type Client interface {
    Publish(ctx context.Context, topic string, msg Message) error
    Subscribe(ctx context.Context, topic string, handler func(Message)) error
    Unsubscribe(ctx context.Context, topic string) error
}

type Message struct {
    From      string          // sender agent/session ID
    Type      string          // "task.assigned" | "task.status" | "task.complete"
    SessionID string
    Payload   json.RawMessage
}
```

### Topic convention
```
topic: "comput3.session.{sessionID}"
```

### Usage pattern
1. Parent agent assigns subtask → `Publish("comput3.session.{childID}", task.assigned)`
2. Child agent executes → `Publish("comput3.session.{parentID}", task.status)`
3. Child complete → `Publish("comput3.session.{parentID}", task.complete{result})`

---

## Integration 3 — KeeperHub (`integrations/keeperhub/`)

### Purpose
On-chain execution retry service. Register critical operations as KeeperHub jobs; if the primary execution fails, KeeperHub re-triggers automatically.

### Interface
```go
type Client interface {
    RegisterJob(ctx context.Context, job Job) (jobID string, error)
    CancelJob(ctx context.Context, jobID string) error
    GetStatus(ctx context.Context, jobID string) (JobStatus, error)
}

type Job struct {
    Name        string
    Target      string          // contract address or webhook URL
    Calldata    []byte          // ABI-encoded call
    MaxRetries  int
    RetryDelay  time.Duration
    ExpiresAt   time.Time
}
```

### Wrapped operations
| Operation | Why | Max retries |
|---|---|---|
| EAS attestation submission | RPC congestion; must land on-chain | 5 |
| `jobsCompleted` increment on ProviderRegistry | Reputation integrity | 3 |
| Escrow release | Payment must be triggered | 3 |

---

## Execution Flow (End-to-End)

```
1. User authenticates
   POST /auth/nonce → sign → POST /auth/verify → JWT

2. User starts session
   POST /sessions
   ├── x402 middleware: verify USDC transferWithAuthorization
   ├── Record payment in DB
   ├── Create Session in DB (state=running)
   └── Spawn agent goroutine

3. Frontend subscribes
   GET /sessions/{id}/stream (WebSocket)

4. Agent: analyze
   Claude → analyze_repo tool
   └── scanner.Scan(githubURL) → DeploymentPlan
   └── 0G: store plan under session key
   └── emit Event{type:"plan"}

5. User confirms
   POST /sessions/{id}/confirm
   └── confirmCh closed → agent unblocks

6. Agent: select provider
   Claude → select_provider tool
   └── chain.GetActiveProviders() → pick cheapest
   └── emit Event{type:"action", tool:"select_provider"}

7. Agent: provision container
   Claude → create_container tool
   └── container.Manager.Create(opts)
       ├── Docker pull + run
       └── LUKS2 volume: vault.img → loop → cryptsetup → mount /app
   └── emit Event{type:"action", tool:"create_container"}

8. Agent: execute steps
   Claude → install_packages / exec_command / configure_network
   └── each: Action logged with SHA256 hash
   └── each: 0G.Append(sessionID, action)
   └── each: emit Event{type:"action"}

9. Agent: AXL coordination (if multi-node)
   AXL.Publish → child agent on another provider
   └── child executes, publishes result back
   └── parent waits on Subscribe

10. Session complete
    └── Merkle root computed over all action hashes
    └── chain.SubmitAttestation(merkleRoot, sessionID, ...)
    └── KeeperHub.RegisterJob("attest", ...) ← retry guarantee
    └── session.State = completed
    └── emit Event{type:"done", attestation_tx}

11. User verifies
    GET /sessions/{id}/audit
    └── returns full action log + Merkle proofs
    └── EAS attestation tx viewable on Monad Testnet explorer
```

---

## Environment Variables

### Backend (`.env`)
```
DATABASE_URL=postgres://...
DOCKER_HOST=unix:///var/run/docker.sock
ANTHROPIC_API_KEY=
OLLAMA_URL=http://localhost:11434
SCAN_MODEL=qwen2.5-coder
AGENT_MODEL=claude-3-5-sonnet-20241022
MONAD_TESTNET_RPC_URL=https://testnet-rpc.monad.xyz
PROVIDER_REGISTRY_ADDRESS=0x...
EAS_SCHEMA_UID=0x...
AGENT_WALLET_PRIVATE_KEY=
JWT_SECRET=
VAULT_MASTER_SECRET=
DEPLOY_DOMAIN=deploy.comput3.xyz
PORT=8080
0G_RPC_URL=
0G_PRIVATE_KEY=
0G_FLOW_ADDRESS=
AXL_ENDPOINT=
KEEPERHUB_API_KEY=
KEEPERHUB_ENDPOINT=
```

---

## Build & Run

### Backend
```bash
cd backend
go mod tidy
go build -o bin/server ./cmd/server
./bin/server
```

### Contracts
```bash
cd contracts
npm install
npx hardhat compile
npx hardhat run scripts/deploy.ts --network monadTestnet
```

### Frontend
```bash
cd frontend
npm install
npm run dev
```

---

## Module 8 — Projects & Deployment Configuration

### Project Model (`store.Project`)
Each team can create multiple named projects, each tied to a repository:
- `id` (UUID), `team_id` (FK), `name`, `repo_url`, `branch` (default `main`)
- `last_prompt` — saved on each deploy so CI/CD can re-run it
- `webhook_secret` — 24 random bytes base64url, returned **only on creation** (never again)
- `auto_deploy` (bool) — if false, GitHub push events are ignored
- `last_deployed_at` — timestamp of most recent CI redeploy

### Per-Project Environment Variables (`store.ProjectEnvVar`)
- `id` (BIGSERIAL), `project_id` (FK CASCADE), `team_id` (FK), `key`, `value` (AES-GCM encrypted)
- `UNIQUE(project_id, key)` — upsert semantics via `INSERT ON CONFLICT DO UPDATE`
- Values are **never** returned by `ListProjectEnvVars` (keys only); use `GetProjectEnvVarValues` in handler layer for decryption

### AES-256-GCM Encryption (`api/crypto.go`)
- Master key = `SHA-256(VAULT_MASTER_SECRET)`
- Encrypt: random 12-byte nonce || AES-GCM ciphertext + 16-byte tag → base64-encoded
- Decrypt: base64 decode, split nonce, GCM.Open — falls back to plaintext for legacy unencrypted values
- When `VAULT_MASTER_SECRET` is empty (dev mode), encrypt/decrypt are pass-through

### Session Creation with Env Vars (`POST /sessions`)
Request body accepts `project_id` (optional) and `env_vars` map (optional).
- If `project_id` is set: loads project's encrypted vars, decrypts them, merges with ad-hoc `env_vars` (ad-hoc wins on collision)
- Updates project's `last_prompt` for CI/CD re-use
- Merged env map is passed to `agent.NewSession` → `buildSystemPrompt(envVars)` writes them into agent instructions

### Agent Env Var Injection (`agent/tools.go`)
`buildSystemPrompt(envVars map[string]string)` appends:
```
## Environment Variables
Write these to /app/.env and pass as env to start_process:
KEY=value
...
```

### GitHub Webhook CI/CD (`POST /webhooks/github/{projectId}`)
1. Body read (1 MB limit), HMAC-SHA256 verified against `project.webhook_secret`
2. Event must be `push` targeting the project's configured `branch`
3. `project.auto_deploy` must be true and `last_prompt` must be non-empty
4. Decrypts project env vars, creates new agent session, runs in goroutine
5. Calls `TouchProjectDeployedAt` regardless of agent outcome

Setup on GitHub: **Settings → Webhooks → Add webhook**
- Payload URL: `https://<your-domain>/api/backend/webhooks/github/<projectId>`
- Content type: `application/json`
- Secret: the `webhook_secret` returned at project creation
- Events: **Just the push event**
