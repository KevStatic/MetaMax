import { defineChain } from "viem";

/** Monad Testnet is EVM-compatible, so wagmi/viem can use it like any EVM chain. */
export const monadTestnet = defineChain({
  id: 10143,
  name: "Monad Testnet",
  nativeCurrency: { name: "Monad", symbol: "MON", decimals: 18 },
  rpcUrls: {
    default: { http: [process.env.NEXT_PUBLIC_MONAD_TESTNET_RPC_URL ?? "https://testnet-rpc.monad.xyz"] },
  },
  blockExplorers: {
    default: { name: "MonadScan", url: "https://testnet.monadscan.com" },
  },
  testnet: true,
});

export const MONAD_EXPLORER = "https://testnet.monadscan.com";
export const MONAD_FAUCET = "https://faucet.monad.xyz";

export const monadTxUrl = (hash: string) => `${MONAD_EXPLORER}/tx/${hash}`;
export const monadAddressUrl = (address: string) => `${MONAD_EXPLORER}/address/${address}`;

/** Trim a hash or address for display without losing its identity. */
export const shortHash = (value: string, lead = 10, tail = 6) =>
  value.length <= lead + tail ? value : `${value.slice(0, lead)}…${value.slice(-tail)}`;

// ── Deployment pipeline ────────────────────────────────────────────────────
// These ids mirror the stage constants the Go agent emits on the WebSocket.

export type StageId =
  | "analyzing"
  | "finding_providers"
  | "comparing"
  | "selecting"
  | "escrow"
  | "encrypting"
  | "executing"
  | "proof"
  | "attesting"
  | "settlement";

export type StageStatus = "pending" | "active" | "done" | "failed" | "skipped";

export type StageDef = {
  id: StageId;
  label: string;
  /** What the step does, in one line, for someone seeing the demo cold. */
  blurb: string;
  /** True when the step normally produces a Monad transaction. */
  onchain: boolean;
};

export const PIPELINE: StageDef[] = [
  { id: "analyzing",         label: "AI analyzing task",   blurb: "The agent reads the workload and drafts a deployment plan",      onchain: false },
  { id: "finding_providers", label: "Finding providers",   blurb: "Reading every staked provider from ProviderRegistry",           onchain: false },
  { id: "comparing",         label: "Comparing nodes",     blurb: "Scoring price, reputation, stake and live latency",             onchain: false },
  { id: "selecting",         label: "Selecting provider",  blurb: "Highest-scoring reachable node wins the job",                   onchain: false },
  { id: "escrow",            label: "Creating escrow",     blurb: "The agent wallet locks MON in DeploymentEscrow",                onchain: true  },
  { id: "encrypting",        label: "Encrypting workload", blurb: "LUKS2 volume created; the key never leaves the vault",          onchain: false },
  { id: "executing",         label: "Executing",           blurb: "The workload runs inside the encrypted container",              onchain: false },
  { id: "proof",             label: "Generating proof",    blurb: "Every action is hashed into a Merkle tree",                     onchain: false },
  { id: "attesting",         label: "Attesting on Monad",  blurb: "The Merkle root is committed to ExecutionAttestation",          onchain: true  },
  { id: "settlement",        label: "Releasing payment",   blurb: "Escrow pays the provider and its reputation counter ticks up",  onchain: true  },
];

/** A stage event as streamed by the backend. */
export type StageEvent = {
  type: "stage";
  stage: StageId;
  status: Exclude<StageStatus, "pending">;
  detail?: string;
  tx_hash?: string;
  explorer_url?: string;
  data?: unknown;
};

/** One scored provider, as returned by /providers/scored and the compare stage. */
export type ScoredProvider = {
  wallet: string;
  endpoint: string;
  price_per_hour_wei: string;
  price_per_hour_mon: string;
  staked_wei: string;
  staked_mon: string;
  jobs_completed: string;
  slash_count: string;
  active: boolean;
  score: number;
  price_score: number;
  reliability_score: number;
  stake_score: number;
  slash_penalty: number;
  latency_ms: number;
  reason: string;
  rank: number;
  explorer_url: string;
};

/** The on-chain receipt for a finished session. */
export type ChainSummary = {
  session_id: string;
  chain_id: number;
  provider?: string;
  provider_explorer_url?: string;
  merkle_root?: string;
  escrow_tx?: string;
  escrow_url?: string;
  attest_tx?: string;
  attest_url?: string;
  settle_tx?: string;
  settle_url?: string;
  reputation_tx?: string;
  reputation_url?: string;
};
