package config

import (
	"log"
	"os"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Port        string
	DatabaseURL string
	DockerHost  string

	// LLM — Groq is the primary model for both scanning and agent
	GroqAPIKey string
	ScanModel  string // Groq model for repo scanning
	AgentModel string // Groq model for deployment agent

	// Deploy domain for subdomain proxy (e.g. "deploy.comput3.xyz")
	DeployDomain string

	// Blockchain — Monad Testnet
	MonadTestnetRPCURL      string
	ProviderRegistryAddress string
	DeploymentEscrowAddress string
	JobAuctionAddress       string
	ExecutionAttestationAddress string
	AgentWalletPrivateKey   string

	// EscrowDepositMON is how much native MON the autonomous agent wallet locks
	// in DeploymentEscrow for the provider it selects, per session.
	EscrowDepositMON string

	// PaymentsDisabled: if true, the x402 paywall in front of session creation
	// is bypassed. Local/demo use only — never enable on a public deployment.
	PaymentsDisabled bool

	// ProviderMode: if true, this node watches for JobPosted events and auto-bids.
	ProviderMode              bool
	ProviderWalletPrivateKey  string

	// Vault — HMAC master secret for per-container LUKS key derivation
	VaultMasterSecret string

	// JWTSecret — signs wallet auth tokens
	JWTSecret string

	// 0G Network — decentralized agent memory
	ZeroG_RPC_URL      string
	ZeroG_PrivateKey   string
	ZeroG_FlowAddress  string

	// Gensyn AXL — cross-node agent pub/sub
	// AXL_Endpoint is the local AXL node API URL (e.g. http://127.0.0.1:9002).
	// AXL_PeerID is the destination peer's 64-char hex ed25519 public key.
	AXL_Endpoint string
	AXL_PeerID   string

	// KeeperHub — on-chain execution reliability
	KeeperHub_Endpoint   string
	KeeperHub_PrivateKey string
}

// Load reads all configuration from environment variables with sensible defaults.
func Load() *Config {
	// The vault secret keys AES-256-GCM secret encryption. Never leave it empty:
	// an empty key made encryptSecret fall back to storing plaintext while the UI
	// still claimed "encrypted at rest". Fall back to a built-in dev key so
	// encryption always happens, but make the misconfiguration loud — the dev key
	// is predictable and unsafe for real data.
	vaultSecret := getEnv("VAULT_MASTER_SECRET", "")
	if vaultSecret == "" {
		vaultSecret = "metamax-insecure-dev-vault-key"
		log.Printf("[config] WARNING: VAULT_MASTER_SECRET is unset — using an insecure built-in dev key so secrets are still encrypted, not stored as plaintext. Set VAULT_MASTER_SECRET before handling real secrets.")
	}

	return &Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://comput3:comput3@localhost:5432/comput3?sslmode=disable"),
		DockerHost:  getEnv("DOCKER_HOST", "unix:///var/run/docker.sock"),

		GroqAPIKey: getEnv("GROQ_API_KEY", ""),
		ScanModel:  getEnv("SCAN_MODEL", "openai/gpt-oss-120b"),
		AgentModel: getEnv("AGENT_MODEL", "openai/gpt-oss-120b"),

		DeployDomain: getEnv("DEPLOY_DOMAIN", ""),

		MonadTestnetRPCURL:      getEnv("MONAD_TESTNET_RPC_URL", "https://testnet-rpc.monad.xyz"),
		ProviderRegistryAddress: getEnv("PROVIDER_REGISTRY_ADDRESS", ""),
		DeploymentEscrowAddress: getEnv("DEPLOYMENT_ESCROW_ADDRESS", ""),
		JobAuctionAddress:       getEnv("JOB_AUCTION_ADDRESS", ""),
		ExecutionAttestationAddress: getEnv("EXECUTION_ATTESTATION_ADDRESS", ""),
		AgentWalletPrivateKey:   getEnv("AGENT_WALLET_PRIVATE_KEY", ""),
		EscrowDepositMON:        getEnv("ESCROW_DEPOSIT_MON", "0.01"),

		PaymentsDisabled:         getEnv("PAYMENTS_DISABLED", "") == "true",
		ProviderMode:             getEnv("PROVIDER_MODE", "") == "true",
		ProviderWalletPrivateKey: getEnv("PROVIDER_WALLET_PRIVATE_KEY", ""),
		VaultMasterSecret:       vaultSecret,
		JWTSecret:               getEnv("JWT_SECRET", getEnv("VAULT_MASTER_SECRET", "comput3-dev-secret")),

		ZeroG_RPC_URL:     getEnv("ZG_RPC_URL", ""),
		ZeroG_PrivateKey:  getEnv("ZG_PRIVATE_KEY", ""),
		ZeroG_FlowAddress: getEnv("ZG_FLOW_ADDRESS", ""),

		AXL_Endpoint: getEnv("AXL_ENDPOINT", ""),
		AXL_PeerID:   getEnv("AXL_PEER_ID", ""),

		KeeperHub_Endpoint:   getEnv("KEEPERHUB_ENDPOINT", ""),
		KeeperHub_PrivateKey: getEnv("KEEPERHUB_PRIVATE_KEY", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
