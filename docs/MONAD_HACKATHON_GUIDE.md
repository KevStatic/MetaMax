# COMPUT3 on Monad: Hackathon Guide

> This is the short operational guide. For the full explanation of the project —
> including what Monad is, what a testnet is, how the proofs and the marketplace
> economics work, and a glossary — see [`../PROJECT_GUIDE.md`](../PROJECT_GUIDE.md).

## What this is

COMPUT3 is a verifiable AI-compute marketplace. A user submits a workload, an agent finds a provider, the workload runs in an encrypted container, and the result is committed on-chain as a Merkle root. Providers stake collateral, receive escrowed payments, and can be slashed for proved misbehavior. Workload contents, source code, and secrets stay off-chain.

## Monad and the testnet

Monad is an EVM-compatible Layer 1, so Solidity, Hardhat, viem, wagmi, standard wallets, and Ethereum JSON-RPC patterns work without rewriting the app. This project targets Monad Testnet:

| Setting | Value |
| --- | --- |
| Chain ID | `10143` |
| Native token | `MON` |
| RPC | `https://testnet-rpc.monad.xyz` |
| Explorer | `https://testnet.monadscan.com` |

Testnet MON has no monetary value. Fund deployer, provider, and agent wallets from the Monad faucet before running the chain flows.

## Marketplace flow

1. A provider registers in `ProviderRegistry` with at least `0.01 MON` stake, endpoint, and hourly MON price.
2. The provider can top up/withdraw stake and toggle availability.
3. `JobAuction` collects bids from active staked providers and awards the lowest valid bid after its visible 30-second window.
4. `DeploymentEscrow` holds MON, sends an upfront payment, streams settlement, and supports refund, dispute, and slash paths.
5. The backend builds a Merkle root from execution action hashes and calls `ExecutionAttestation.attest()` on Monad.

`ExecutionAttestation` is a project-owned native attestation contract. It stores only session/team hashes, Merkle root, timestamp, and attester—not private workload data.

## Contract responsibilities

| Contract | Job |
| --- | --- |
| `ProviderRegistry` | Registration, stake, availability, reputation counter, slashing |
| `JobAuction` | Posting, bidding, deterministic lowest-bid selection |
| `DeploymentEscrow` | MON escrow, streaming settlement, refunds, disputes |
| `ExecutionAttestation` | Merkle-root commitment and revocation |

## Deployment checklist

1. Copy `.env.example` to `.env`; set `DEPLOYER_PRIVATE_KEY` and `MONAD_TESTNET_RPC_URL`.
2. Run `./scripts/deploy-contracts.sh`.
3. Put its output into `.env`: `PROVIDER_REGISTRY_ADDRESS`, `DEPLOYMENT_ESCROW_ADDRESS`, `JOB_AUCTION_ADDRESS`, and `EXECUTION_ATTESTATION_ADDRESS`.
4. Set `PROVIDER_ENDPOINT` and run `./scripts/register-provider.sh`.
5. Restart frontend/backend so they receive the updated addresses.
6. Verify every deployment on MonadScan. The deploy script attempts this when `MONADSCAN_API_KEY` is set; otherwise submit the source, compiler `0.8.24`, optimizer enabled with 200 runs, and constructor arguments in the explorer UI.

The blank values in `frontend/lib/contracts/deployments.json` are intentional. They prevent the old Sepolia deployment from being used; fill addresses only after your team’s Monad deployment.

## Demo narration

Open `/launch` and click **Deploy my AI workload**. The pipeline renders itself:

`AI analyzing task → Finding providers → Comparing nodes → Selecting provider → Creating escrow → Encrypting workload → Executing → Generating proof → Attesting on Monad → Releasing payment`

Four of those steps produce a real transaction — escrow, attestation, reputation and
settlement — and each row carries its MonadScan link. Open at least one during the
demo; that is what separates this from a console animation.

Before demoing, confirm:

- `GET /agent/wallet` shows a non-zero MON balance (otherwise the on-chain stages skip)
- `GET /providers/scored` lists at least one provider with `latency_ms >= 0`

## Important boundaries

- Never commit `.env` or private keys.
- The agent wallet needs MON to fund escrow, attest and settle. Fund it from the faucet.
- Escrow binds to the provider the agent actually selected; `ESCROW_DEPOSIT_MON` controls the amount.
- Session IDs map to the escrow's `bytes32` key with **SHA-256** on both sides (`chain.SessionIDToJobID` in Go, `sessionIdToBytes32` in `lib/useEscrow.ts`). Change one and you must change the other, or deposits and releases address different escrow entries.
- The repository’s x402/USDC experimental route is not a Monad-native settlement primitive. The marketplace settlement path for this migration is native-MON escrow.
