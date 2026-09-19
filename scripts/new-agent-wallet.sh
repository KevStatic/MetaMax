#!/usr/bin/env bash
# Generates a fresh testnet wallet and optionally writes the key into .env.
# Runs entirely on your machine; nothing is transmitted.
#
#   ./scripts/new-agent-wallet.sh --write
#       generate AGENT_WALLET_PRIVATE_KEY into .env (key never displayed)
#
#   ./scripts/new-agent-wallet.sh --write DEPLOYER_PRIVATE_KEY
#       same, for a different variable
#
#   ./scripts/new-agent-wallet.sh
#       just print the key and address
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

ARGS=""
if [ "${1:-}" = "--write" ]; then
  [ -f "$ROOT/.env" ] || { echo "no .env at $ROOT/.env" >&2; exit 1; }
  cp "$ROOT/.env" "$ROOT/.env.bak.$(date +%Y%m%d%H%M%S)"
  ARGS="-env /repo/.env -var ${2:-AGENT_WALLET_PRIVATE_KEY}"
fi

docker run --rm \
  -v "$ROOT/scripts/agentwallet":/app \
  -v "$ROOT":/repo \
  -v metamax-gocache:/go/pkg/mod \
  -w /app golang:1.25-alpine \
  sh -c "go mod tidy >/dev/null 2>&1 && go run main.go $ARGS"
