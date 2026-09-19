# meta-max.xyz production deployment

Target: Monad mainnet, chain ID 143, native currency MON.
USDC: 0x754704Bc059F8C67012fEd69BC8A327a5aafb603 (Circle).

## Server and DNS
Use a Linux host with Docker Compose and ports 80/443 open. Point the A records for meta-max.xyz and www.meta-max.xyz to that host. Remove conflicting AAAA records unless IPv6 is configured. Caddy obtains and renews HTTPS certificates automatically.

Copy .env.example to .env. Set POSTGRES_PASSWORD, JWT_SECRET and VAULT_MASTER_SECRET to random values. Set GROQ_API_KEY, AGENT_WALLET_PRIVATE_KEY and NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID. The local .env generated during setup is ignored by Git; do not commit it.

## Contracts
Set DEPLOYER_PRIVATE_KEY for a funded Monad wallet and MONAD_RPC_URL. Run npm ci in contracts, then npm run deploy:monad. Set PROVIDER_REGISTRY_ADDRESS, DEPLOYMENT_ESCROW_ADDRESS and JOB_AUCTION_ADDRESS from contracts/deployments.json. Set their NEXT_PUBLIC_ equivalents for frontend development outside Docker. Docker forwards the server addresses into the frontend build.

EAS requires an actual Monad deployment: set EAS_CONTRACT_ADDRESS and EAS_SCHEMA_REGISTRY_ADDRESS, run npm run eas:register, then set EAS_SCHEMA_UID. No EAS deployment address is assumed. Register a compute provider with npm run become-provider before submitting workloads.

## Start
From the repository root:

    docker compose -f docker-compose.yml -f compose.production.yml up -d --build

Check https://meta-max.xyz and https://meta-max.xyz/api/backend/health. Confirm wallet connections use Monad, then test a funded session and its transaction receipt. Restart policies keep services running across host restarts.

Optional GitHub repository integration requires GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET, with callback https://meta-max.xyz/api/github/callback. Rebuild the frontend after changing any public setting or contract address. Optional 0G, AXL and KeeperHub integrations require their own credentials.

DEPLOY_DOMAIN is optional and needs separate wildcard DNS/proxy configuration for workload subdomains; the supplied Caddyfile serves the app and API only. The existing GitHub workflow is a webhook integration and does not provision this host.

## Verification limits
Local frontend checks do not prove production deployment, wallet transactions, paid AI execution, EAS or encrypted workload execution. Verify these with the production environment and funded wallets before treating the service as fully operational.

## Local verification results
Frontend production build passed; dashboard, settings, payments, provider registration and sign-in returned HTTP 200. The source and rendered pages passed the requested reference scan. Monad RPC returned 0x8f. Payment domain/header checks and wrong-network rejection passed. All three contracts compiled and deployed on the ephemeral Hardhat chain; these are not production addresses. Compose configuration validated. Docker engine was unavailable and meta-max.xyz DNS returned ENOTFOUND during setup.

Docker recovery completed: stale socket directories were preserved as backups under the local Docker runtime paths. All four Compose services are healthy. The frontend serves localhost:3000; the local API port is 18080 because 8080 is occupied. The frontend API proxy returned HTTP 200 with status ok. Public domain deployment still requires DNS/hosting and production credentials.

## GitHub deployment workflow

To enable automatic deployment on pushes to main, configure the repository secrets METAMAX_WEBHOOK_URL and METAMAX_WEBHOOK_SECRET, then set the repository variable METAMAX_DEPLOY_ENABLED to true. The workflow stays disabled until configured.
