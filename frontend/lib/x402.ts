/**
 * x402 client helpers.
 *
 * Flow:
 *   1. Call API → 402 → parse x402PaymentRequired body
 *   2. buildTransferTypedData() → get EIP-712 typed data to sign
 *   3. User signs via wagmi useSignTypedData
 *   4. makePaymentHeader(signature, typedData) → base64 JSON for X-PAYMENT header
 *   5. Retry original request with X-PAYMENT header
 */

import { isAddress } from "viem";

export interface X402Accepts {
  scheme: string;
  network: string;
  maxAmountRequired: string;
  resource: string;
  payTo: string;
  maxTimeoutSeconds: number;
  asset: string;
  extra?: { name?: string; version?: string };
}

export interface X402PaymentRequired {
  x402Version: number;
  error: string;
  accepts: X402Accepts[];
}

export const MONAD_TESTNET_CHAIN_ID = 10143;

export interface TransferTypedData {
  domain: {
    name: string;
    version: string;
    chainId: number;
    verifyingContract: `0x${string}`;
  };
  types: {
    TransferWithAuthorization: Array<{ name: string; type: string }>;
  };
  primaryType: "TransferWithAuthorization";
  message: {
    from: `0x${string}`;
    to: `0x${string}`;
    value: bigint;
    validAfter: bigint;
    validBefore: bigint;
    nonce: `0x${string}`;
  };
}

function randomNonce(): `0x${string}` {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return ("0x" + Array.from(bytes).map((b) => b.toString(16).padStart(2, "0")).join("")) as `0x${string}`;
}

/** Build EIP-712 typed data for USDC transferWithAuthorization. */
export function buildTransferTypedData(
  accept: X402Accepts,
  from: `0x${string}`
): TransferTypedData {
  // The server fills payTo from its agent wallet and asset from the USDC
  // contract. If either is missing the node is misconfigured, and signing would
  // fail deep inside viem with an opaque InvalidAddressError — so say so here.
  if (!isAddress(accept.payTo)) {
    throw new Error(
      "This node cannot accept payment: its agent wallet is not configured, so it " +
        "returned an empty payTo address. Set a valid 32-byte AGENT_WALLET_PRIVATE_KEY " +
        "on the backend, or set PAYMENTS_DISABLED=true to run without payments."
    );
  }
  if (!isAddress(accept.asset)) {
    throw new Error("This node returned an invalid payment asset address.");
  }

  const name = accept.extra?.name ?? "USD Coin";
  const version = accept.extra?.version ?? "2";
  const value = BigInt(accept.maxAmountRequired);
  const now = BigInt(Math.floor(Date.now() / 1000));

  return {
    domain: {
      name,
      version,
      chainId: MONAD_TESTNET_CHAIN_ID,
      verifyingContract: accept.asset as `0x${string}`,
    },
    types: {
      TransferWithAuthorization: [
        { name: "from", type: "address" },
        { name: "to", type: "address" },
        { name: "value", type: "uint256" },
        { name: "validAfter", type: "uint256" },
        { name: "validBefore", type: "uint256" },
        { name: "nonce", type: "bytes32" },
      ],
    },
    primaryType: "TransferWithAuthorization",
    message: {
      from,
      to: accept.payTo as `0x${string}`,
      value,
      validAfter: now - 60n,
      validBefore: now + BigInt(accept.maxTimeoutSeconds),
      nonce: randomNonce(),
    },
  };
}

/** Encode the signed typed data into a base64 X-PAYMENT header value. */
export function makePaymentHeader(
  signature: `0x${string}`,
  typedData: TransferTypedData
): string {
  const payload = {
    x402Version: 1,
    scheme: "exact",
    network: `eip155:${MONAD_TESTNET_CHAIN_ID}`,
    payload: {
      signature,
      authorization: {
        from: typedData.message.from,
        to: typedData.message.to,
        value: typedData.message.value.toString(),
        validAfter: typedData.message.validAfter.toString(),
        validBefore: typedData.message.validBefore.toString(),
        nonce: typedData.message.nonce,
      },
    },
  };
  return btoa(JSON.stringify(payload));
}
