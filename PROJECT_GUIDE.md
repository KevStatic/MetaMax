# COMPUT3 on Monad — the complete guide

Everything you need to understand this project: what it does, what Monad is, what a
testnet is, how the pieces fit together, and how to run and demo it.

If you read only one paragraph: **COMPUT3 is a marketplace where an AI agent rents
compute from strangers and pays them automatically — and every important step is
recorded on the Monad blockchain so nobody has to be trusted.** You click one button,
the agent finds a provider, locks up money, runs your workload in an encrypted
container, publishes a cryptographic proof of what it did, and releases the payment.

---

## Table of contents

1. [The problem this solves](#1-the-problem-this-solves)
2. [Blockchain concepts you need (plain English)](#2-blockchain-concepts-you-need-plain-english)
3. [What Monad is, and why this project uses it](#3-what-monad-is-and-why-this-project-uses-it)
4. [System architecture](#4-system-architecture)
5. [The four smart contracts](#5-the-four-smart-contracts)
6. [The deployment pipeline, stage by stage](#6-the-deployment-pipeline-stage-by-stage)
7. [How verification actually works](#7-how-verification-actually-works)
8. [The marketplace economics](#8-the-marketplace-economics)
9. [Running it yourself](#9-running-it-yourself)
10. [The demo script](#10-the-demo-script)
11. [API reference](#11-api-reference)
12. [Repository map](#12-repository-map)
13. [What is real and what is not](#13-what-is-real-and-what-is-not)
14. [Glossary](#14-glossary)

---

## 1. The problem this solves

When you run an AI workload on someone else's computer, you have to trust them on
three separate things:

1. **That they ran it at all.** You get an output back. How do you know it came from
   the model and the code you asked for, rather than something cheaper?
2. **That they didn't look at your data.** Your source code, your API keys, your
   training data all sit on their disk.
3. **That you'll be paid / that you'll get what you paid for.** If they take the
   money and vanish, or you take the compute and don't pay, there is no referee.

Centralised clouds solve this with brand reputation — you trust AWS because AWS is
big. That doesn't work for a marketplace of anonymous GPU owners.

COMPUT3 replaces trust with three mechanisms:

| Problem | COMPUT3's answer |
| --- | --- |
| Did they really run it? | Every action the agent takes is hashed; the hashes form a **Merkle tree**; the root is published on Monad. Tampering with any step changes the root. |
| Did they see my data? | The workload runs on a **LUKS2-encrypted volume** whose key is derived per-container and never stored on the provider's disk. |
| Will anyone get paid? | Money sits in an **escrow smart contract**. The provider is paid on proof of completion; the user is refunded if the job fails; a provider that misbehaves gets **slashed**. |

None of these require trusting COMPUT3 either — the contracts are public and the
proofs are verifiable by anyone.

---

## 2. Blockchain concepts you need (plain English)

Skip this if you're already comfortable with EVM chains.

**Blockchain.** A database that many computers keep identical copies of. Nobody owns
it; changes are agreed by consensus, and once written, history can't quietly change.
Useful here because it gives us a referee that neither the user nor the provider
controls.

**Smart contract.** A program that lives on the blockchain. Its code is public, and
it runs exactly as written — nobody can reach in and change what it does. Our escrow
is a smart contract: it physically cannot pay the provider unless the release
condition is met.

**EVM (Ethereum Virtual Machine).** The standard execution environment for smart
contracts, originally from Ethereum. "EVM-compatible" means a chain runs the same
bytecode, so the same Solidity code, the same tools (Hardhat, viem, wagmi) and the
same wallets (MetaMask) work without changes. This is why porting COMPUT3 to Monad
was a config change and not a rewrite.

**Chain ID.** A number identifying which network you're on — Ethereum mainnet is `1`,
Monad Testnet is `10143`. Transactions are signed with the chain ID baked in, so a
transaction meant for one chain can't be replayed on another.

**Native token / gas.** Every chain has a built-in currency used to pay for
computation. Ethereum's is ETH; Monad's is **MON**. Every transaction costs a small
amount of it ("gas"), which is what stops people from spamming the network.

**Testnet.** A full copy of the network used for development, where the tokens are
free and worth nothing. It behaves like the real thing — same code, same tools, same
explorer — so you can build and demo without spending real money. Monad Testnet has
chain ID `10143` and a **faucet**, a website that gives you free test MON so your
wallet can pay gas. **Testnet MON has no monetary value; never treat it as an asset.**

**Wallet / private key / address.** A private key is a secret number. From it you
derive a public address (`0x…`), which is your identity on-chain. Signing a
transaction with the private key proves it came from that address. In this project
three different wallets appear: the **deployer** (publishes the contracts), the
**agent wallet** (the autonomous wallet that pays providers), and the **provider
wallet** (a compute seller). Keep them separate; never commit any of them.

**Transaction hash (tx hash).** A unique id for a submitted transaction, like
`0x9f2c…`. Paste it into a block explorer and you see exactly what happened. This is
why the demo attaches a MonadScan link to every on-chain step — you don't have to
believe the UI, you can check the chain.

**Block explorer.** A website that lets you browse the chain in a readable way. For
Monad Testnet this project uses **MonadScan** (`https://testnet.monadscan.com`).

**Staking and slashing.** Staking = locking up your own money as collateral to be
allowed to participate. Slashing = taking part of that collateral away when you are
proved to have misbehaved. Together they make honesty the profitable choice: a
provider that cheats loses more than it gains.

**Escrow.** Money held by a neutral third party until conditions are met. Here the
neutral third party is a smart contract, which is better than a human escrow agent
because it has no discretion.

**Attestation.** A short, signed statement published on-chain saying "this thing
happened". We attest the Merkle root of an execution — not the data itself, which
stays private.

---

## 3. What Monad is, and why this project uses it

**Monad is a Layer 1 blockchain that is fully EVM-compatible but built for much
higher throughput than Ethereum.** Its two headline ideas are **parallel execution**
(independent transactions run at the same time instead of strictly one after another,
with conflicts resolved afterwards) and a **pipelined consensus** design, where the
stages of producing a block overlap rather than waiting on each other. The result is
a chain that targets dramatically more transactions per second and much faster blocks
than Ethereum L1, while keeping bytecode-level compatibility.

> Check Monad's current published throughput and block-time figures before quoting
> exact numbers in a pitch — they change as the network evolves.

**Why it matters for COMPUT3 specifically.** This is not a project that sends one
transaction a day. A single deployment produces an escrow transaction, an attestation,
a reputation update and a settlement — and a busy marketplace multiplies that by every
job, every bid and every dispute. On a slow or expensive chain, the per-job on-chain
overhead would swamp the value of a short compute job, and you'd be pushed into
batching everything off-chain, which defeats the point. A fast, cheap, EVM-compatible
chain lets each job carry its own on-chain receipt.

The compatibility matters just as much: Solidity, Hardhat, viem/wagmi, MetaMask and
standard JSON-RPC all work unchanged, so the whole migration was contract
redeployment plus configuration.

### Monad Testnet settings used by this project

| Setting | Value |
| --- | --- |
| Network name | Monad Testnet |
| Chain ID | `10143` |
| Native token | `MON` (test MON — no monetary value) |
| RPC endpoint | `https://testnet-rpc.monad.xyz` |
| Block explorer | `https://testnet.monadscan.com` |
| Faucet | `https://faucet.monad.xyz` |

These live in exactly two places in the code: [`backend/internal/chain/explorer.go`](backend/internal/chain/explorer.go)
for the Go side and [`frontend/lib/monad.ts`](frontend/lib/monad.ts) for the browser side.
Change them there and the whole app follows.

**Adding Monad Testnet to MetaMask:** Settings → Networks → Add network manually, then
enter the RPC URL, chain ID `10143`, symbol `MON` and the explorer URL above. Then
visit the faucet to get test MON.

---

## 4. System architecture

```
                    ┌──────────────────────────────────┐
   Browser  ───────▶│  Next.js frontend (frontend/)    │
   + MetaMask       │  /launch  one-button demo        │
                    │  /deploy  step-by-step flow      │
                    │  /provider  register, stake      │
                    └───────────────┬──────────────────┘
                          REST + WebSocket
                                    │
                    ┌───────────────▼──────────────────┐
                    │  Go backend (backend/)           │
                    │  ├ agent/    the AI deploy agent │
                    │  ├ chain/    Monad transactions  │
                    │  ├ container/ Docker + LUKS      │
                    │  ├ api/      HTTP + WS handlers  │
                    │  └ store/    Postgres            │
                    └───┬──────────────────────────┬───┘
                        │                          │
             JSON-RPC (signed txs)        Docker Engine API
                        │                          │
        ┌───────────────▼──────────┐   ┌───────────▼────────────┐
        │  Monad Testnet (10143)   │   │ Encrypted containers    │
        │  ProviderRegistry        │   │ LUKS2 volume per job    │
        │  DeploymentEscrow        │   └─────────────────────────┘
        │  JobAuction              │
        │  ExecutionAttestation    │
        └──────────────────────────┘
```

**Frontend** — Next.js App Router, wagmi + RainbowKit for wallet connection, viem for
contract calls. It never holds a private key; the user signs in MetaMask.

**Backend** — Go. It holds the *agent wallet* and is the only component that sends
autonomous transactions. It runs the agent loop (an LLM calling tools), drives Docker,
computes proofs, and streams every event to the browser over a WebSocket.

**Contracts** — Solidity 0.8.24, deployed with Hardhat, verified on MonadScan.

**Optional integrations** — 0G Network (decentralised storage for the action log),
Gensyn AXL (agent-to-agent messaging), KeeperHub (scheduled on-chain follow-ups). All
three degrade to no-ops when unconfigured, so the core flow works without them.

---

## 5. The four smart contracts

All four live in [`contracts/contracts/`](contracts/contracts/).

### ProviderRegistry.sol — who is allowed to sell compute

A provider calls `register(endpoint, pricePerHour)` and sends at least `MIN_STAKE`
(0.01 MON) along with it. That stake is their collateral. The contract then tracks:

- `endpoint` — the HTTPS URL of their node
- `pricePerHour` — their asking price in wei
- `stakedAmount` — collateral currently locked
- `jobsCompleted` — **reputation**: incremented by the backend after each successful job
- `slashCount` — how many times they've been punished
- `active` — whether they're accepting work

Providers can `stake()` more, `unstake()` some back (dropping below `MIN_STAKE`
auto-deactivates them), and `deactivate()` / `reactivate()` to control availability.
`getActiveProviders()` returns everyone currently available — this is the query the
agent runs when hunting for a node.

`slash(provider, evidence)` takes 50% of the stake. `evidence` is a 32-byte hash
committing to the off-chain proof — COMPUT3 uses the session's Merkle root, so the
reason for a slash is itself auditable.

### DeploymentEscrow.sol — where the money sits

Two payment modes:

**Simple escrow.** `deposit(sessionId, provider)` with MON attached locks the funds.
`release(sessionId)` pays the provider minus a 10% protocol fee. `refund(sessionId)`
lets the user take their money back after a 24-hour lockup if nothing happened.

**Streaming.** `startSession(sessionId, provider, ratePerSecond)` pays 20% upfront and
escrows the rest, which then drips to the provider second by second as
`releasePayment()` is called. `stopSession()` settles what's owed and refunds the
remainder. This suits long-running workloads where neither side wants to front the
entire cost.

**Dispute and slashing.** `dispute(sessionId)` freezes a pending escrow;
`resolveDispute(sessionId, toProvider)` sends it one way or the other;
`slashProvider(sessionId, evidence)` stops the session, refunds the user, and calls
into the registry to slash the provider's stake.

### JobAuction.sol — competitive price discovery

Instead of the agent picking from a list, it can `postJob()` with resource
requirements and a ceiling price, wait a **30-second bid window** while providers call
`submitBid()`, then `closeAuction()`. The lowest valid bid from an active, staked
provider wins. If nobody bids, it falls back to direct registry selection.

### ExecutionAttestation.sol — the proof of what happened

`attest(sessionId, teamId, merkleRoot)` records a commitment on-chain. Deliberately
minimal: it stores only hashes, a timestamp, and who attested. **No workload data, no
source code, no logs ever touch the chain.** Anyone holding the off-chain action log
can recompute the root and compare it to what's on Monad.

---

## 6. The deployment pipeline, stage by stage

This is what the **"Deploy my AI workload"** button on [`/launch`](frontend/app/launch/page.tsx)
runs. The backend emits a `stage` event as it enters and leaves each step, and the UI
draws one row per stage. Stages marked ⛓ produce a real Monad transaction with a
MonadScan link.

| # | Stage | What actually happens |
| --- | --- | --- |
| 1 | **AI analyzing task** | The LLM agent reads the prompt and repo, detects the framework, and drafts a deployment plan (containers, ports, commands, RAM/CPU). In autopilot the agent approves its own plan. |
| 2 | **Finding providers** | `getActiveProviders()` is read from ProviderRegistry — or, with auctions enabled, a job is posted ⛓ and a 30-second bid window opens. |
| 3 | **Comparing nodes** | Every candidate is scored (see below) and health-checked in parallel with a 3-second `/health` probe. |
| 4 | **Selecting provider** | The highest-scoring *reachable* node wins. The UI shows the whole ranking table, so the choice is visible, not asserted. |
| 5 | ⛓ **Creating escrow** | The **agent's own wallet** calls `deposit()` and locks MON for that specific provider. No human signs this — it is an agent-to-provider payment. |
| 6 | **Encrypting workload** | A 512 MB LUKS2 volume is created, its key derived as `HMAC-SHA256(masterSecret, containerID)`, mounted into the container at `/app`, and the temporary keyfile deleted. |
| 7 | **Executing** | The agent clones the repo, installs packages, writes env files and starts processes — each tool call recorded as an `Action` with a SHA-256 hash. |
| 8 | **Generating proof** | All action hashes are combined into a binary Merkle tree; the root summarises the entire run in 32 bytes. |
| 9 | ⛓ **Attesting on Monad** | The root is committed via `ExecutionAttestation.attest()`. |
| 10 | ⛓ **Releasing payment** | `recordJobCompleted()` bumps the provider's reputation, then `release()` pays them from escrow. |

Steps 9 and 10 run inside a **finalizer hook** that fires *before* the WebSocket
closes, specifically so their transaction links stream into the UI live rather than
appearing only after a page reload. See [`backend/internal/agent/monad_flow.go`](backend/internal/agent/monad_flow.go).

If a stage can't run — no agent wallet key, no deployed escrow address — it is marked
**skipped** rather than failed, and the pipeline continues. The demo therefore still
works on a node with no chain configuration, it just shows fewer transactions.

---

## 7. How verification actually works

This is the heart of the project, so it's worth understanding properly.

**Step 1 — hash every action.** Each tool call the agent makes becomes an `Action`
record: index, tool name, inputs, result, timestamp. It's hashed:

```
hash = SHA256(index | tool | JSON(input) | JSON(result) | ISO8601(timestamp))
```

**Step 2 — build a Merkle tree.** The action hashes are the leaves. Adjacent pairs are
hashed together, then those results are paired and hashed, and so on until a single
**Merkle root** remains. (An odd leaf at any level is duplicated.)

```
        ROOT
       /    \
     H12     H34
    /  \    /   \
   H1  H2  H3   H4        ← one leaf per action
```

**Step 3 — publish the root.** Just the 32-byte root goes on Monad. Cheap, private,
and permanent.

**Step 4 — verify later.** Given one action and its **Merkle proof** (the sibling
hashes on the path to the root), anyone can recompute the root and compare it to the
on-chain value. If a single byte of a single action was altered after the fact, the
recomputed root won't match.

The `/audit` page renders these proofs per action, with each sibling labelled `left:`
or `right:` so the recomputation is reproducible by hand.

**What this does and does not prove.** It proves the action log wasn't altered after
attestation, and it binds a specific session to a specific root at a specific time. It
does **not** prove the provider's CPU honestly executed the instructions — that would
need a TEE or a zero-knowledge proof of execution. The honest framing for a demo is:
*tamper-evident execution logs with on-chain commitment, plus staking and slashing to
make cheating expensive.*

---

## 8. The marketplace economics

### Provider scoring

The agent doesn't just pick the cheapest node — cheapest is exactly what an attacker
would offer. [`backend/internal/chain/scoring.go`](backend/internal/chain/scoring.go)
scores each provider out of 100:

```
score = 0.45 × priceScore        (cheapest in the set = 100, most expensive = 0)
      + 0.30 × reliabilityScore  (100 × jobs / (jobs + 5) — saturating, so
      |                           experience helps but can't dominate)
      + 0.25 × stakeScore        (collateral, normalised against the largest stake)
      − 25 × slashCount          (every past punishment is a flat penalty)
```

Then every endpoint is health-checked in parallel; unreachable nodes drop to the
bottom of the ranking regardless of score, because an offline node can't run anything.
The same ranking is exposed at `GET /providers/scored`, so a provider can see exactly
why they were or weren't chosen.

The feedback loop closes on-chain: finishing a job increments `jobsCompleted`, which
raises `reliabilityScore`, which wins more jobs. Getting slashed does the reverse.

### Why a provider stays honest

| Provider action | Consequence |
| --- | --- |
| Completes the job | Paid from escrow (minus 10% fee); `jobsCompleted` +1; ranks higher next time |
| Goes offline mid-job | Health check fails; escrow refunded after lockup; no payment |
| Provably cheats | `slashProvider()` — half the stake gone, `slashCount` +1, −25 score, likely deactivated |

A provider must put up at least 0.01 MON before earning anything, and the penalty for
cheating is designed to exceed the take from any single job.

---

## 9. Running it yourself

### Prerequisites

- Node.js 20+ and npm
- Go 1.25+
- Docker (for containers, Postgres, and the workload sandbox)
- A wallet with test MON from `https://faucet.monad.xyz`

### Step 1 — configure

```bash
cp .env.example .env
```

Fill in at minimum:

| Variable | What it is |
| --- | --- |
| `GROQ_API_KEY` | The LLM key the agent and repo scanner use |
| `DEPLOYER_PRIVATE_KEY` | Wallet that publishes the contracts (needs test MON) |
| `AGENT_WALLET_PRIVATE_KEY` | The autonomous agent wallet (needs test MON) |
| `JWT_SECRET`, `VAULT_MASTER_SECRET` | Two different random 64-char hex strings — `openssl rand -hex 32` |
| `POSTGRES_PASSWORD` | Any password for the local database |

**Never commit `.env`.** It is gitignored; keep it that way.

### Step 2 — deploy the contracts

```bash
cd contracts && npm install
cd .. && bash scripts/deploy-contracts.sh
```

This publishes all four contracts to Monad Testnet, saves the addresses to
`contracts/deployments.json`, and attempts MonadScan verification when
`MONADSCAN_API_KEY` is set. Copy the four addresses into `.env`:

```
PROVIDER_REGISTRY_ADDRESS=0x…
DEPLOYMENT_ESCROW_ADDRESS=0x…
JOB_AUCTION_ADDRESS=0x…
EXECUTION_ATTESTATION_ADDRESS=0x…
```

Then export the ABIs to the frontend:

```bash
cd contracts && npm run export:abis
```

If automatic verification fails, verify by hand on MonadScan with: compiler
**0.8.24**, optimizer **enabled**, **200 runs**, and the constructor arguments printed
by the deploy script.

### Step 3 — register at least one provider

The marketplace is empty until somebody is selling. On the machine that will act as a
provider:

```bash
export PROVIDER_ENDPOINT=https://your-node.example.com
bash scripts/register-provider.sh
```

Or use the `/provider/register` page in the UI. Without a registered provider the
pipeline still runs, but the provider stages show as skipped and it falls back to the
local node.

### Step 4 — run

```bash
docker compose up --build
```

Frontend on `http://localhost:3000`, backend on `http://localhost:8080`.

To run the pieces separately during development:

```bash
cd backend  && go run ./cmd/server
cd frontend && npm install && npm run dev
```

### Health check before demoing

1. `curl localhost:8080/health` → `{"status":"ok"}`
2. `curl localhost:8080/agent/wallet` → shows the agent address and a **non-zero** MON
   balance. If it's zero, the on-chain stages will skip — go to the faucet.
3. `curl localhost:8080/providers/scored` → at least one provider, `latency_ms ≥ 0`.

---

## 10. The demo script

Open `/launch`, connect the wallet, and press **Deploy my AI workload**.

While it runs, narrate:

> "The agent is reading the workload and writing its own deployment plan — nobody
> approved this, it's on autopilot.
>
> Now it's reading the provider registry **on Monad**. Eight nodes are registered;
> here's the score for each one — price, reputation, stake, latency. It's not taking
> the cheapest, it's taking the best.
>
> It picked Provider #4 — and here's the escrow transaction. **That's the agent's own
> wallet paying a stranger, no human signature anywhere.** Click it — that's MonadScan,
> that's the real chain.
>
> Workload's running in an encrypted container now; the provider can't read it.
>
> Execution done. Every step was hashed into a Merkle tree, and here's the root going
> on-chain as an attestation — that's the proof this exact sequence ran.
>
> And payment released. The provider's reputation counter just went up, which means
> they'll rank higher on the next job."

**Every ⛓ row is a clickable MonadScan link.** Open one. The credibility of the whole
demo rests on the audience seeing that these are real transactions on a real chain,
not console output.

Have ready as backup: `/providers/scored` in a browser tab (the raw ranking), the
`/audit` page for the session (the Merkle proofs), and the four contract pages on
MonadScan (verified source).

---

## 11. API reference

Endpoints added or changed for the Monad migration are marked **new**.

### Sessions

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/sessions` | Create a session. Body accepts `prompt`, `repo_url`, `project_id`, `env_vars`, and **new** `autopilot` (skips the human approval gate). |
| `GET` | `/sessions/{id}/stream` | WebSocket. Emits `stage`, `action`, `message`, `plan`, `done`, `error`. |
| `POST` | `/sessions/{id}/confirm` | Approve the plan (not needed in autopilot). |
| `GET` | `/sessions/{id}/audit` | Full action log with per-action Merkle proofs. |
| `GET` | **`/sessions/{id}/chain`** | Every Monad transaction the session produced, with explorer links. |
| `POST` | `/sessions/{id}/attest` | Re-submit the attestation. |
| `POST` | `/sessions/{id}/release-escrow` | Release payment manually. |
| `POST` | **`/sessions/{id}/dispute`** | Freeze the escrow; `{"resolve":true,"to_provider":bool}` settles it. |
| `POST` | **`/sessions/{id}/slash`** | Slash the session's provider, committing the Merkle root as evidence. |

### Marketplace

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/providers/active` | Raw registry read. |
| `GET` | **`/providers/scored`** | The full scored ranking with latency probes and score breakdown. |
| `POST` | `/providers/bid` | Submit an auction bid. |
| `GET` | **`/agent/wallet`** | Agent wallet address, live MON balance, and all four contract addresses with explorer links. |

### The WebSocket `stage` event

```json
{
  "type": "stage",
  "stage": "escrow",
  "status": "done",
  "detail": "0.01 MON escrowed for the provider",
  "tx_hash": "0x9f2c…",
  "explorer_url": "https://testnet.monadscan.com/tx/0x9f2c…"
}
```

`status` is one of `active`, `done`, `failed`, `skipped`. Stage ids are listed in
`PIPELINE` in [`frontend/lib/monad.ts`](frontend/lib/monad.ts) and as constants in
[`backend/internal/agent/session.go`](backend/internal/agent/session.go) — the two
lists must stay in sync.

---

## 12. Repository map

```
comput3/
├── contracts/            Solidity + Hardhat
│   ├── contracts/        ProviderRegistry, DeploymentEscrow, JobAuction,
│   │                     ExecutionAttestation
│   └── scripts/          deploy.ts, export-abis.ts, become-provider.ts
│
├── backend/              Go API + agent
│   ├── cmd/server/       main.go — wiring and startup
│   └── internal/
│       ├── agent/        session.go    the LLM tool loop + Merkle proofs
│       │                 monad_flow.go pipeline stages, scoring, escrow, settlement
│       │                 delegation.go agent-to-agent subtasks over AXL
│       │                 tools.go      tool definitions given to the model
│       ├── chain/        explorer.go   chain id + MonadScan links (single source)
│       │                 scoring.go    provider scoring and latency probes
│       │                 wallet.go     agent wallet address + MON balance
│       │                 escrow.go     deposit / release / dispute / slash
│       │                 provider.go   registry reads + reputation writes
│       │                 auction.go    post / bid / close
│       │                 attestation.go Merkle-root commitment
│       ├── api/          handlers.go   HTTP + WebSocket
│       ├── container/    Docker lifecycle + LUKS encryption
│       └── store/        Postgres
│
├── frontend/             Next.js
│   ├── app/launch/       ★ the one-button demo
│   ├── app/deploy/       step-by-step flow with human approval
│   ├── app/provider/     register, settings, stake, availability
│   ├── app/audit/        Merkle proof viewer
│   └── lib/monad.ts      chain config + pipeline definitions
│
├── scripts/              deploy-contracts.sh, register-provider.sh
├── docs/                 architecture, checklist, implementation notes
└── PROJECT_GUIDE.md      this file
```

---

## 13. What is real and what is not

Be straight about this — judges ask, and the honest answer is strong enough.

**Real, on-chain, verifiable:**
- All four contracts deployed and verifiable on Monad Testnet
- Provider registration, staking, unstaking, availability toggling
- Competitive auction with a real 30-second bid window
- Escrow funded by the agent's own wallet, released on completion
- Merkle roots attested on-chain, with per-action proofs in the UI
- Reputation counters that feed back into the next selection
- Dispute and slashing paths, callable and wired to endpoints

**Real but off-chain:**
- LUKS2 container encryption (a host-level mechanism, not a chain one)
- Provider health probing and scoring (computed by the backend from on-chain data)
- The action log itself (only its root is on-chain — by design)

**Conditional — degrades to a no-op when unconfigured:**
- 0G Network storage (`ZG_*`), Gensyn AXL delegation (`AXL_*`), KeeperHub (`KEEPERHUB_*`)

**Honest limitations:**
- Attestation proves the log wasn't tampered with after the fact; it does not prove
  the provider's hardware executed faithfully. That needs a TEE or zk proofs.
- Dispute resolution ends in an owner decision — a real deployment needs a proper
  arbitration mechanism.
- The x402/USDC payment route in the repo predates the Monad migration; **native MON
  escrow is the settlement path** for this build.
- Testnet only. None of the MON involved is worth anything.

---

## 14. Glossary

| Term | Meaning |
| --- | --- |
| **ABI** | The machine-readable description of a contract's functions, used to encode calls |
| **Attestation** | A short on-chain statement that something happened |
| **Chain ID** | The number identifying a network — `10143` for Monad Testnet |
| **EVM** | Ethereum Virtual Machine — the standard smart-contract runtime |
| **Escrow** | Funds held by a contract until a release condition is met |
| **Faucet** | A site that gives out free testnet tokens |
| **Gas** | The fee paid in the native token for executing a transaction |
| **LUKS2** | The Linux disk-encryption format used for workload volumes |
| **Merkle root** | A single hash summarising a whole set of hashed items |
| **Merkle proof** | The sibling hashes needed to recompute the root from one leaf |
| **MON** | Monad's native token (test MON on the testnet — no value) |
| **MonadScan** | The block explorer used by this project |
| **Nonce** | A per-wallet counter that orders transactions and prevents replays |
| **RPC** | The JSON API endpoint through which the app talks to the chain |
| **Slashing** | Confiscating part of a misbehaving participant's stake |
| **Staking** | Locking your own funds as collateral to be allowed to participate |
| **Testnet** | A free, disposable copy of the network used for development |
| **Tx hash** | The unique id of a transaction, lookupable on an explorer |
| **wei** | The smallest unit — 1 MON = 10¹⁸ wei |
