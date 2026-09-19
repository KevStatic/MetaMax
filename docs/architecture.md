# MetaMax — System Architecture

## Overview

MetaMax is structured as a layered system where each layer has a single, well-defined responsibility. Layers communicate via explicit interfaces — no layer reaches across to another's internals.

```
┌──────────────────────────────────────────────────────────────┐
│                        USER INTERFACE                        │
│          Next.js frontend — wallet auth, task submission,    │
│          live execution stream, verification dashboard       │
└─────────────────────────────┬────────────────────────────────┘
                              │ HTTPS + WebSocket
┌─────────────────────────────▼────────────────────────────────┐
│                         API LAYER                            │
│     Go/Chi HTTP server — routes, auth middleware,            │
│     x402 payment verification, session management           │
└────────┬────────────────────┬────────────────────────────────┘
         │                    │
┌────────▼───────┐   ┌────────▼───────────────────────────────┐
│  AGENT LAYER   │   │           PROVIDER LAYER               │
│                │   │                                        │
│  Claude LLM    │   │  Container Manager (Docker SDK)        │
│  Structured    │   │  LUKS2 encrypted volume per session    │
│  tool use      │   │  SSH gateway, workspace provisioning   │
│  Session state │   │  Package execution environment         │
└────────┬───────┘   └────────┬───────────────────────────────┘
         │                    │
┌────────▼────────────────────▼────────────────────────────────┐
│                     VERIFICATION LAYER                       │
│   Action log (SHA256 per step) → Merkle root computation     │
│   EAS attestation submission on Monad Testnet                 │
│   Audit trail queryable by session ID                        │
└─────────────────────────────┬────────────────────────────────┘
                              │
┌─────────────────────────────▼────────────────────────────────┐
│                     BLOCKCHAIN LAYER                         │
│   ProviderRegistry.sol — staking, reputation, selection      │
│   DeploymentEscrow.sol — streaming payment, slash            │
│   EAS contract — on-chain attestations                       │
│   Monad Testnet (low-latency L2)                              │
└─────────────────────────────┬────────────────────────────────┘
                              │
┌─────────────────────────────▼────────────────────────────────┐
│                    PARTNER INTEGRATIONS                      │
│   0G Network      — decentralized agent memory & log store  │
│   Gensyn AXL      — cross-node agent-to-agent messaging     │
│   KeeperHub       — on-chain execution retry guarantees     │
└──────────────────────────────────────────────────────────────┘
```

---

## Component Breakdown

### 1. User Interface Layer

**Purpose:** Entry point for users. Handles authentication, task submission, live monitoring, and result verification.

**Key responsibilities:**
- Wallet connect + SIWE (Sign-In with Ethereum)
- x402 USDC payment signing (EIP-3009 `transferWithAuthorization`)
- WebSocket subscription to live execution event stream
- Execution plan confirmation flow
- Attestation and audit log viewer

**Communication:** HTTPS REST + WebSocket to API layer. Direct RPC reads from Monad Testnet for on-chain data.

---

### 2. API Layer

**Purpose:** HTTP server that coordinates all subsystems. Owns routing, auth, and session lifecycle.

**Key responsibilities:**
- JWT issuance after SIWE verification
- x402 middleware: verifies USDC payment before starting compute
- Session CRUD and streaming WebSocket bridge
- Vault key gate: blockchain-gated decryption key endpoint
- Proxy for subdomain-based workspace routing (`*.deploy.comput3.xyz`)

**Auth model:** Wallet address is the identity. JWT issued after signature verification. No email/password.

---

### 3. Agent Layer

**Purpose:** The AI brain. Given a user task, the agent plans and executes it using structured tools.

**Key responsibilities:**
- Receives user prompt + GitHub URL (or task description)
- Calls `analyze_repo` → scanner produces `DeploymentPlan`
- Calls `generate_deployment_plan` → presents plan to user, blocks for confirmation
- Calls `select_provider` → reads on-chain registry, picks cheapest active provider
- Calls `create_container`, `install_packages`, `configure_network`, etc.
- Emits structured `Event` objects to WebSocket stream per action
- Each `Action` is assigned a SHA256 hash at execution time

**Agent model:** Anthropic Claude with structured tool use. Tools are the only execution interface — the agent cannot run arbitrary commands.

**State:** `Session` struct holds the full action log, plan, selected provider, and channel for user confirmation.

---

### 4. Provider Layer (Container Manager)

**Purpose:** Executes workloads in isolated, encrypted containers.

**Key responsibilities:**
- Pull Docker images and create containers with resource limits (RAM, CPU)
- Set up LUKS2 encrypted volume (`vault.img`) and mount at `/app` inside container
- Vault key is derived from `HMAC(masterSecret, containerID)` — provider never sees raw key
- Provision VS Code Server or Jupyter workspace on demand
- SSH gateway for direct terminal access
- Track port mappings and expose via subdomain proxy

**Encryption model:**
```
masterSecret (server-side secret)
       │
       ▼ HMAC-SHA256(containerID)
   vault key (AES-256)
       │
       ▼ written to temp keyfile → cryptsetup luksFormat → keyfile deleted
   encrypted /app volume (accessible only while container is running)
```

---

### 5. Verification Layer

**Purpose:** Ensures every execution is auditable and tamper-evident.

**Key responsibilities:**
- Each `Action` executed by the agent is SHA256-hashed (tool + input + result + timestamp)
- All action hashes for a session form a Merkle tree
- Merkle root is submitted as an EAS attestation on Monad Testnet at session end
- Users can verify any action was included in the session by providing a Merkle proof

**Hash format:**
```
actionHash = SHA256(index | tool | JSON(input) | JSON(result) | ISO8601(timestamp))
```

**Merkle construction:** Binary Merkle tree, left-to-right, SHA256 for internal nodes.

---

### 6. Blockchain Layer

**Purpose:** On-chain economic coordination and trust anchoring.

**Contracts (Monad Testnet):**

| Contract | Role |
|---|---|
| `ProviderRegistry` | Provider registration, staking (0.01 MON min), reputation, slash authority |
| `DeploymentEscrow` | MON escrow with streaming payments (wei/second); 20% upfront release |
| `JobAuction` | 30-second bid window; routes to cheapest provider; fallback to MetaMax node |
| EAS (predeploy) | Attestation storage; session Merkle roots anchored here |

---

## Partner Integration Placement

### 0G — Decentralized Memory

**Where:** Agent layer + Verification layer

**What:** Agent session state (conversation history, action log, plan) is persisted to 0G Network using a KV interface. This means:
- Agent can resume sessions across restarts
- Action logs are available without relying on the provider's database
- Multiple agents can share state across nodes

**Interface:** `integrations/0g/` — KV client wrapping 0G's storage API.

---

### Gensyn AXL — Agent-to-Agent Messaging

**Where:** Agent layer (multi-node scenarios)

**What:** When a task requires coordination across multiple providers (e.g. frontend agent on node A, backend agent on node B), agents communicate using AXL's pub/sub messaging. Each session gets a topic derived from session ID.

**Interface:** `integrations/axl/` — publish/subscribe client wrapping AXL's messaging protocol.

**Message types:**
- `task.assigned` — parent agent delegates subtask
- `task.status` — subtask agent reports progress
- `task.complete` — subtask result returned to parent

---

### KeeperHub — Execution Reliability

**Where:** API layer + Blockchain layer

**What:** Critical steps that must not be dropped:
- EAS attestation submission (if Monad Testnet is congested)
- `jobsCompleted` counter update on ProviderRegistry
- Escrow release after session completion

These are wrapped as KeeperHub jobs so that if the primary execution fails (e.g. RPC timeout), KeeperHub re-triggers the transaction automatically.

**Interface:** `integrations/keeperhub/` — job registration and status polling.

---

## Core Guarantees

### Privacy
- Provider node never has access to plaintext user data
- LUKS2 key is derived server-side and never transmitted to the provider
- Container filesystem is encrypted at rest; plaintext exists only in memory during execution

### Verifiability
- Every action is hashed at execution time
- Merkle root of the full action log is submitted on-chain
- Any third party can verify a specific action was part of a session using a Merkle proof
- EAS attestation timestamp provides an immutable record of when execution completed

### Decentralization
- Providers are permissionlessly registered on-chain with staked collateral
- Provider selection is deterministic from on-chain data (cheapest active provider)
- No single provider is trusted; economic incentives enforce honest behavior
- Agent memory (0G) and communication (AXL) are decentralized — no central coordinator

---

## Data Flow Diagram

```
User submits task
       │
       ▼
POST /sessions  ──────────────────── x402 USDC payment verified
       │
       ▼
Session created in DB
Agent goroutine spawned
       │
       ├──► 0G: store session context
       │
       ▼
Claude: analyze_repo
  └── scanner LLM → DeploymentPlan
       │
       ▼
Claude: generate_deployment_plan
  └── Event{type:"plan"} → WebSocket → User
       │
       ▼
User confirms plan
POST /sessions/{id}/confirm
       │
       ▼
Claude: select_provider
  └── eth_call → ProviderRegistry → cheapest active provider
       │
       ▼
Claude: create_container
  └── Docker create + LUKS2 volume + mount /app
       │
       ▼
Claude: install_packages / configure_network / exec_command
  └── each step: Action{hash=SHA256(...)} logged
  └── Event{type:"action"} → WebSocket → User
  └── 0G: append action to session log
       │
       ▼
Session complete
  └── Merkle root computed from all action hashes
  └── EAS.attest(sessionID, merkleRoot, provider, teamID)
  └── KeeperHub: ensure attestation lands if RPC fails
       │
       ▼
Event{type:"done", attestation_tx} → WebSocket → User
```
