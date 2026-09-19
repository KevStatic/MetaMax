#!/usr/bin/env bash
# register-provider.sh  Register the deployer wallet as a provider on-chain
# Usage: ./scripts/register-provider.sh
#
# Reads PRIVATE_KEY, RPC_URL, PROVIDER_REGISTRY_ADDRESS from the environment (or .env).

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
: "${PROVIDER_REGISTRY_ADDRESS:?PROVIDER_REGISTRY_ADDRESS is required - run deploy-contracts.sh first}"
: "${MONAD_TESTNET_RPC_URL:=https://testnet-rpc.monad.xyz}"

cd "$CONTRACTS_DIR"

echo "==> Registering provider with ProviderRegistry at $PROVIDER_REGISTRY_ADDRESS"
DEPLOYER_PRIVATE_KEY="$DEPLOYER_PRIVATE_KEY" \
MONAD_TESTNET_RPC_URL="$MONAD_TESTNET_RPC_URL" \
PROVIDER_REGISTRY_ADDRESS="$PROVIDER_REGISTRY_ADDRESS" \
  npx hardhat run scripts/become-provider.ts --network monadTestnet

echo " Provider registered."
