#!/usr/bin/env bash
# deploy-contracts.sh — Compile and deploy ProviderRegistry, DeploymentEscrow, JobAuction
# Usage: ./scripts/deploy-contracts.sh
#
# Reads PRIVATE_KEY and RPC_URL from the environment (or .env).
# Writes deployed addresses to contracts/deployments/monadTestnet.json

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONTRACTS_DIR="$ROOT_DIR/contracts"

# Source .env if it exists
if [[ -f "$ROOT_DIR/.env" ]]; then
  set -o allexport
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +o allexport
fi

: "${DEPLOYER_PRIVATE_KEY:?DEPLOYER_PRIVATE_KEY is required}"
: "${MONAD_TESTNET_RPC_URL:=https://testnet-rpc.monad.xyz}"

cd "$CONTRACTS_DIR"

echo "==> Installing contract dependencies…"
npm install --silent

echo "==> Compiling contracts…"
npx hardhat compile --quiet

echo "==> Deploying to Monad Testnet ($MONAD_TESTNET_RPC_URL)…"
DEPLOYER_PRIVATE_KEY="$DEPLOYER_PRIVATE_KEY" \
MONAD_TESTNET_RPC_URL="$MONAD_TESTNET_RPC_URL" \
  npx hardhat run scripts/deploy.ts --network monadTestnet

echo "==> Exporting ABIs…"
npx hardhat run scripts/export-abis.ts --network monadTestnet

echo ""
echo "✓ Deployment complete. Update .env with the addresses printed above."
