#!/usr/bin/env bash
# register-eas-schema.sh — Register the audit log EAS schema on Monad Testnet
# Usage: ./scripts/register-eas-schema.sh
#
# Reads PRIVATE_KEY, RPC_URL, EAS_CONTRACT_ADDRESS from the environment (or .env).
# Prints the schema UID — copy it to EAS_SCHEMA_UID in .env.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONTRACTS_DIR="$ROOT_DIR/contracts"

if [[ -f "$ROOT_DIR/.env" ]]; then
  set -o allexport
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +o allexport
fi

: "${DEPLOYER_PRIVATE_KEY:?DEPLOYER_PRIVATE_KEY is required}"
: "${MONAD_TESTNET_RPC_URL:=https://testnet-rpc.monad.xyz}"
: "${EAS_CONTRACT_ADDRESS:?EAS_CONTRACT_ADDRESS is required}"

cd "$CONTRACTS_DIR"

echo "==> Registering EAS schema…"
DEPLOYER_PRIVATE_KEY="$DEPLOYER_PRIVATE_KEY" \
MONAD_TESTNET_RPC_URL="$MONAD_TESTNET_RPC_URL" \
EAS_CONTRACT_ADDRESS="$EAS_CONTRACT_ADDRESS" \
  npx hardhat run scripts/register-eas-schema.ts --network monadTestnet

echo "✓ Copy the schema UID above into EAS_SCHEMA_UID in your .env"
