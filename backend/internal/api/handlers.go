package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"

	"github.com/shreejaykurhade/MetaMax/backend/integrations/keeperhub"
	"github.com/shreejaykurhade/MetaMax/backend/internal/agent"
	"github.com/shreejaykurhade/MetaMax/backend/internal/auth"
	"github.com/shreejaykurhade/MetaMax/backend/internal/chain"
	"github.com/shreejaykurhade/MetaMax/backend/internal/config"
	"github.com/shreejaykurhade/MetaMax/backend/internal/container"
	"github.com/shreejaykurhade/MetaMax/backend/internal/scanner"
	"github.com/shreejaykurhade/MetaMax/backend/internal/store"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Server holds all dependencies for the HTTP API.
type Server struct {
	cfg        *config.Config
	db         *store.Store
	mgr        *container.Manager
	sc         *scanner.Scanner
	authSvc    *auth.Service
	keeper     keeperhub.Client
	zeroG      agent.ZeroGClient
	axl        agent.AXLClient

	mu       sync.RWMutex
	sessions map[string]*agent.Session
}

// NewServer creates a new API server with all dependencies wired.
func NewServer(
	cfg *config.Config,
	db *store.Store,
	mgr *container.Manager,
	sc *scanner.Scanner,
	authSvc *auth.Service,
	keeper keeperhub.Client,
	zeroG agent.ZeroGClient,
	axl agent.AXLClient,
) *Server {
	return &Server{
		cfg:      cfg,
		db:       db,
		mgr:      mgr,
		sc:       sc,
		authSvc:  authSvc,
		keeper:   keeper,
		zeroG:    zeroG,
		axl:      axl,
		sessions: make(map[string]*agent.Session),
	}
}

// Router builds and returns the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	r.Get("/health", s.handleHealth)

	// Auth
	r.Post("/auth/nonce", s.handleAuthNonce)
	r.Post("/auth/verify", s.handleAuthVerify)

	// Account (wallet-based, no JWT required for GET — wallet in query param)
	r.Get("/account", s.handleGetAccount)
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Post("/account", s.handleUpdateAccount)
	})

	// Sessions
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Post("/sessions", s.x402Middleware(s.handleCreateSession))
		r.Get("/sessions/{id}", s.handleGetSession)
		r.Post("/sessions/{id}/confirm", s.handleConfirmSession)
		r.Post("/sessions/{id}/attest", s.handleReAttest)
		r.Post("/sessions/{id}/release-escrow", s.handleReleaseEscrow)
		r.Post("/sessions/{id}/dispute", s.handleDisputeSession)
		r.Post("/sessions/{id}/slash", s.handleSlashProvider)
		r.Get("/sessions/{id}/chain", s.handleSessionChain)
		r.Get("/sessions/{id}/audit", s.handleGetAudit)
	})

	// The live session stream is reached over WebSocket, which cannot carry an
	// Authorization header from a browser — see wsAuthMiddleware.
	r.Group(func(r chi.Router) {
		r.Use(s.wsAuthMiddleware)
		r.Get("/sessions/{id}/stream", s.handleStreamSession)
	})

	// Repository scan — powers the framework picker on /deploy
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Post("/repos/scan", s.handleScanRepo)
	})

	// Provider discovery, scoring & bidding
	r.Get("/providers/active", s.handleGetProviders)
	r.Get("/providers/scored", s.handleGetScoredProviders)
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Post("/providers/bid", s.handleSubmitBid)
	})

	// Autonomous agent wallet — address, MON balance, deployed contracts
	r.Get("/agent/wallet", s.handleAgentWallet)

	// Team resources (JWT required)
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Get("/teams/{id}/workspaces", s.handleListWorkspaces)
		r.Get("/teams/{id}/sessions", s.handleListTeamSessions)
		r.Get("/teams/{id}/attestations", s.handleListTeamAttestations)
	})

	// Attestation detail (JWT required)
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Get("/attestations/{sessionId}", s.handleGetAttestation)
	})

	// Vault
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Get("/vault/nonce", s.handleVaultNonce)
		r.Post("/vault/key", s.handleVaultKey)
	})

	// Payments
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Get("/payments", s.handleListPayments)
	})

	// Secrets
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Get("/secrets", s.handleListSecrets)
		r.Post("/secrets", s.handleCreateSecret)
		r.Delete("/secrets/{id}", s.handleDeleteSecret)
	})

	// Projects + per-project env vars
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Get("/projects", s.handleListProjects)
		r.Post("/projects", s.handleCreateProject)
		r.Get("/projects/{id}", s.handleGetProject)
		r.Put("/projects/{id}", s.handleUpdateProject)
		r.Delete("/projects/{id}", s.handleDeleteProject)
		r.Get("/projects/{id}/env", s.handleListProjectEnv)
		r.Post("/projects/{id}/env", s.handleUpsertProjectEnv)
		r.Delete("/projects/{id}/env/{envId}", s.handleDeleteProjectEnv)
	})

	// GitHub webhooks — public endpoint, HMAC-verified per project
	r.Post("/webhooks/github/{projectId}", s.handleGitHubWebhook)

	// Wrap with subdomain proxy: requests to <containerID>.DEPLOY_DOMAIN are
	// reverse-proxied to the container's mapped port on localhost.
	return s.subdomainProxyMiddleware(r)
}

// --- Auth ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleAuthNonce(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Address == "" {
		jsonError(w, "address required", http.StatusBadRequest)
		return
	}
	nonce := s.authSvc.IssueNonce(req.Address)
	jsonOK(w, map[string]string{"nonce": nonce})
}

// handleAuthVerify verifies a SIWE-style message+signature.
// For this implementation we accept the wallet address + nonce + raw signature.
// A production build should parse the full EIP-4361 message.
func (s *Server) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address   string `json:"address"`
		Nonce     string `json:"nonce"`
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.Address == "" || req.Nonce == "" || req.Signature == "" {
		jsonError(w, "address, nonce, and signature required", http.StatusBadRequest)
		return
	}
	// Consume first so a nonce can never be replayed, then prove the caller
	// actually controls the address they claim.
	if err := s.authSvc.ConsumeNonce(req.Address, req.Nonce); err != nil {
		jsonError(w, fmt.Sprintf("nonce invalid: %v", err), http.StatusUnauthorized)
		return
	}
	if err := auth.VerifyPersonalSign(req.Address, req.Nonce, req.Signature); err != nil {
		log.Printf("[api] auth verify rejected for %s: %v", req.Address, err)
		jsonError(w, "signature verification failed", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	team, err := s.db.GetOrCreateTeamByWallet(ctx, req.Address)
	if err != nil {
		log.Printf("[api] GetOrCreateTeamByWallet: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	token, err := s.authSvc.IssueJWT(req.Address, team.ID)
	if err != nil {
		jsonError(w, "could not issue token", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"token": token, "team_id": team.ID})
}

// --- Sessions ---

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	var req struct {
		Prompt    string            `json:"prompt"`
		RepoURL   string            `json:"repo_url"`
		ProjectID string            `json:"project_id"`
		EnvVars   map[string]string `json:"env_vars"` // ad-hoc env vars for this session
		// Autopilot runs the full pipeline with no human approval step —
		// the one-button "Deploy my AI workload" demo.
		Autopilot bool `json:"autopilot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Prompt == "" {
		jsonError(w, "prompt required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Merge env vars: project-level (persisted) + request-level (ad-hoc) vars.
	// Ad-hoc vars override project vars if keys collide.
	mergedEnv := map[string]string{}
	if req.ProjectID != "" {
		proj, err := s.db.GetProject(ctx, req.ProjectID, teamID)
		if err == nil {
			// Update last_prompt on the project so CI/CD can reuse it
			_ = s.db.UpdateProject(ctx, proj.ID, teamID, proj.Name, proj.RepoURL,
				proj.Branch, req.Prompt, proj.AutoDeploy)
			// Load project env vars (encrypted) and decrypt
			encVars, err := s.db.GetProjectEnvVarValues(ctx, req.ProjectID, teamID)
			if err == nil {
				for k, enc := range encVars {
					if plain, err := decryptSecret(s.cfg.VaultMasterSecret, enc); err == nil {
						mergedEnv[k] = plain
					}
				}
			}
		}
	}
	for k, v := range req.EnvVars {
		mergedEnv[k] = v
	}

	sessionID := newUUID()
	if err := s.db.CreateSession(ctx, &store.Session{
		ID:     sessionID,
		TeamID: teamID,
		Prompt: req.Prompt,
		State:  "running",
	}); err != nil {
		log.Printf("[api] CreateSession: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	sess := agent.NewSession(s.sessionConfig(sessionID, teamID, mergedEnv, req.Autopilot))
	// The finalizer commits the execution proof and settles payment on Monad
	// while the WebSocket is still open, so every transaction link streams live.
	sess.Finalizer = s.chainFinalizer(sessionID, teamID)

	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	// Run agent in background
	go func() {
		runCtx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
		defer cancel()
		if err := sess.Run(runCtx, req.Prompt); err != nil {
			log.Printf("[api] session %s failed: %v", sessionID, err)
		}

		// Persist action log and update session state in DB
		if err := s.db.UpsertActionLog(runCtx, sessionID, teamID, sess.Actions); err != nil {
			log.Printf("[api] UpsertActionLog: %v", err)
		}
		if err := s.db.UpdateSessionState(runCtx, sessionID, string(sess.State)); err != nil {
			log.Printf("[api] UpdateSessionState: %v", err)
		}
		if err := s.db.UpdateSessionMerkleRoot(runCtx, sessionID, sess.MerkleRoot, sess.AttestTxHash); err != nil {
			log.Printf("[api] UpdateSessionMerkleRoot: %v", err)
		}
	}()

	jsonOK(w, map[string]string{"id": sessionID, "session_id": sessionID})
}

// sessionConfig assembles the agent session configuration from server config.
func (s *Server) sessionConfig(sessionID, teamID string, envVars map[string]string, autopilot bool) agent.SessionConfig {
	return agent.SessionConfig{
		ID:                 sessionID,
		TeamID:             teamID,
		Manager:            s.mgr,
		Scanner:            s.sc,
		ZeroG:              s.zeroG,
		AXL:                s.axl,
		GroqAPIKey:         s.cfg.GroqAPIKey,
		Model:              s.cfg.AgentModel,
		RPCURL:             s.cfg.MonadTestnetRPCURL,
		RegistryAddress:    s.cfg.ProviderRegistryAddress,
		AuctionAddress:     s.cfg.JobAuctionAddress,
		EscrowAddress:      s.cfg.DeploymentEscrowAddress,
		AttestationAddress: s.cfg.ExecutionAttestationAddress,
		AgentPrivKey:       s.cfg.AgentWalletPrivateKey,
		EscrowDepositMON:   s.cfg.EscrowDepositMON,
		DeployDomain:       s.cfg.DeployDomain,
		EnvVars:            envVars,
		Autopilot:          autopilot,
	}
}

// chainFinalizer returns the hook the agent runs after computing its Merkle
// root: attest on Monad, bump provider reputation, release escrow, then record
// the attestation in Postgres and hand the follow-up to KeeperHub.
func (s *Server) chainFinalizer(sessionID, teamID string) func(context.Context, *agent.Session) {
	return func(ctx context.Context, sess *agent.Session) {
		sess.SettleAndAttest(ctx)

		if sess.AttestTxHash == "" {
			return
		}
		monadScanURL := chain.MonadTxURL(sess.AttestTxHash)
		_ = s.db.CreateAttestation(ctx, &store.Attestation{
			SessionID:      sessionID,
			TxHash:         sess.AttestTxHash,
			AttestationUID: "",
			MerkleRoot:     sess.MerkleRoot,
			SchemaUID:      s.cfg.ExecutionAttestationAddress,
			EASScanURL:     monadScanURL,
		})
		_ = s.db.UpdateSessionMerkleRoot(ctx, sessionID, sess.MerkleRoot, sess.AttestTxHash)

		_, _ = s.keeper.RegisterJob(ctx, keeperhub.Job{
			Name:      "confirm-attestation",
			SessionID: sessionID,
			Payload: map[string]any{
				"tx_hash":     sess.AttestTxHash,
				"merkle_root": sess.MerkleRoot,
				"explorer":    monadScanURL,
			},
		})
		_, _ = s.keeper.RegisterJob(ctx, keeperhub.Job{
			Name:      "release-escrow",
			SessionID: sessionID,
			Payload: map[string]any{
				"session_id":      sessionID,
				"escrow_contract": s.cfg.DeploymentEscrowAddress,
				"rpc_url":         s.cfg.MonadTestnetRPCURL,
			},
		})
	}
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess := s.lookupSession(id)
	if sess == nil {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]any{
		"id":          sess.ID,
		"team_id":     sess.TeamID,
		"state":       sess.State,
		"action_count": len(sess.Actions),
		"merkle_root": sess.MerkleRoot,
	})
}

func (s *Server) handleConfirmSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess := s.lookupSession(id)
	if sess == nil {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}
	sess.Confirm()
	jsonOK(w, map[string]string{"status": "confirmed"})
}

// handleReAttest manually re-triggers a Monad execution attestation.
func (s *Server) handleReAttest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	if s.cfg.AgentWalletPrivateKey == "" || s.cfg.ExecutionAttestationAddress == "" {
		jsonError(w, "attestation not configured on this node", http.StatusServiceUnavailable)
		return
	}

	dbSess, err := s.db.GetSession(ctx, id)
	if err != nil {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}
	if dbSess.MerkleRoot == "" {
		jsonError(w, "session has no merkle root — run the session first", http.StatusConflict)
		return
	}

	merkleArr, err := chain.HexToBytes32(dbSess.MerkleRoot)
	if err != nil {
		jsonError(w, "invalid merkle root stored for session", http.StatusInternalServerError)
		return
	}

	result, err := chain.SubmitMonadAttestation(ctx,
		s.cfg.MonadTestnetRPCURL, s.cfg.AgentWalletPrivateKey,
		s.cfg.ExecutionAttestationAddress, id, dbSess.TeamID, merkleArr)
	if err != nil {
		log.Printf("[api] handleReAttest SubmitMonadAttestation: %v", err)
		jsonError(w, fmt.Sprintf("attestation failed: %v", err), http.StatusBadGateway)
		return
	}

	uid := ""
	monadScanURL := chain.MonadTxURL(result.TxHash)

	attest := &store.Attestation{
		SessionID:      id,
		TxHash:         result.TxHash,
		AttestationUID: uid,
		MerkleRoot:     dbSess.MerkleRoot,
		SchemaUID:      s.cfg.ExecutionAttestationAddress,
		EASScanURL:     monadScanURL,
	}
	_ = s.db.CreateAttestation(ctx, attest)
	_ = s.db.UpdateSessionMerkleRoot(ctx, id, dbSess.MerkleRoot, result.TxHash)

	jsonOK(w, map[string]any{
		"session_id":      id,
		"tx_hash":         result.TxHash,
		"attestation_uid": uid,
		"explorer_url":    monadScanURL,
		"merkle_root":     dbSess.MerkleRoot,
	})
}

// handleReleaseEscrow is called by KeeperHub after session completion to release
// locked ETH from the DeploymentEscrow contract to the provider.
// The endpoint is protected by JWT so only authenticated clients (or KeeperHub
// using a bearer token) can trigger it.
func (s *Server) handleReleaseEscrow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	if s.cfg.AgentWalletPrivateKey == "" || s.cfg.DeploymentEscrowAddress == "" {
		jsonError(w, "escrow not configured on this node", http.StatusServiceUnavailable)
		return
	}

	dbSess, err := s.db.GetSession(ctx, id)
	if err != nil {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}
	if dbSess.State != "completed" {
		jsonError(w, "session not completed", http.StatusConflict)
		return
	}

	// Escrow keys are derived the same way everywhere: keccak256(sessionID).
	sessionID := chain.SessionIDToJobID(id)

	txHash, err := chain.ReleaseEscrow(
		ctx,
		s.cfg.MonadTestnetRPCURL,
		s.cfg.AgentWalletPrivateKey,
		s.cfg.DeploymentEscrowAddress,
		sessionID,
	)
	if err != nil {
		log.Printf("[api] ReleaseEscrow session=%s: %v", id, err)
		jsonError(w, fmt.Sprintf("release failed: %v", err), http.StatusBadGateway)
		return
	}

	jsonOK(w, map[string]string{
		"status":       "released",
		"session_id":   id,
		"tx_hash":      txHash,
		"explorer_url": chain.MonadTxURL(txHash),
	})
}

// handleAgentWallet reports the autonomous agent wallet that pays providers,
// attests proofs and settles escrow — address, live MON balance, explorer link.
func (s *Server) handleAgentWallet(w http.ResponseWriter, r *http.Request) {
	// A node with no agent key configured is a normal state on a fresh install,
	// so report it as such instead of a 500 the UI has to swallow.
	var info any
	configured := false
	if got, err := chain.AgentWallet(r.Context(), s.cfg.MonadTestnetRPCURL, s.cfg.AgentWalletPrivateKey); err != nil {
		log.Printf("[api] agent wallet unavailable: %v", err)
	} else {
		info, configured = got, true
	}
	jsonOK(w, map[string]any{
		"configured": configured,
		"wallet":     info,
		"rpc_url":  s.cfg.MonadTestnetRPCURL,
		"explorer": chain.MonadExplorerBase,
		"contracts": map[string]any{
			"provider_registry":     contractRef(s.cfg.ProviderRegistryAddress),
			"deployment_escrow":     contractRef(s.cfg.DeploymentEscrowAddress),
			"job_auction":           contractRef(s.cfg.JobAuctionAddress),
			"execution_attestation": contractRef(s.cfg.ExecutionAttestationAddress),
		},
		"escrow_deposit_mon": s.cfg.EscrowDepositMON,
	})
}

func contractRef(address string) map[string]string {
	return map[string]string{
		"address":      address,
		"explorer_url": chain.MonadAddressURL(address),
	}
}

// handleGetScoredProviders returns every active provider ranked by the same
// scoring the agent uses: price, reputation, stake, slash history, latency.
func (s *Server) handleGetScoredProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	emptyRanked := func(note string) {
		jsonOK(w, map[string]any{
			"count":     0,
			"providers": []chain.ScoredProvider{},
			"note":      note,
		})
	}
	if s.cfg.ProviderRegistryAddress == "" {
		emptyRanked("provider registry not configured on this node")
		return
	}
	providers, err := chain.GetActiveProviders(ctx, s.cfg.MonadTestnetRPCURL, s.cfg.ProviderRegistryAddress)
	if err != nil {
		log.Printf("[api] scored providers: registry query failed: %v", err)
		emptyRanked("registry query failed")
		return
	}
	ranked := chain.ScoreProviders(providers)
	if r.URL.Query().Get("probe") != "false" {
		ranked = chain.ProbeLatency(ctx, ranked, 3*time.Second)
	}
	jsonOK(w, map[string]any{
		"count":     len(ranked),
		"providers": ranked,
		"weights": map[string]float64{
			"price":       chain.WeightPrice,
			"reliability": chain.WeightReliability,
			"stake":       chain.WeightStake,
		},
		"slash_penalty": chain.SlashPenalty,
	})
}

// handleSessionChain returns every Monad transaction a session produced.
func (s *Server) handleSessionChain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if sess := s.lookupSession(id); sess != nil {
		jsonOK(w, sess.ChainSummary())
		return
	}
	dbSess, err := s.db.GetSession(r.Context(), id)
	if err != nil {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]any{
		"session_id":  id,
		"chain_id":    chain.MonadTestnetChainID,
		"merkle_root": dbSess.MerkleRoot,
		"attest_tx":   dbSess.AttestationTx,
		"attest_url":  chain.MonadTxURL(dbSess.AttestationTx),
	})
}

// handleDisputeSession freezes a session's escrow pending resolution. This is
// the entry point to the dispute flow: the funds stop moving until the owner
// calls resolve, and the session's Merkle root is the evidence commitment.
func (s *Server) handleDisputeSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	if s.cfg.AgentWalletPrivateKey == "" || s.cfg.DeploymentEscrowAddress == "" {
		jsonError(w, "escrow not configured on this node", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Resolve    bool `json:"resolve"`     // true → settle an existing dispute
		ToProvider bool `json:"to_provider"` // resolution direction
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	sessionID := chain.SessionIDToJobID(id)

	var (
		txHash string
		err    error
		status string
	)
	if req.Resolve {
		status = "resolved"
		txHash, err = chain.ResolveDispute(ctx, s.cfg.MonadTestnetRPCURL,
			s.cfg.AgentWalletPrivateKey, s.cfg.DeploymentEscrowAddress, sessionID, req.ToProvider)
	} else {
		status = "disputed"
		txHash, err = chain.DisputeEscrow(ctx, s.cfg.MonadTestnetRPCURL,
			s.cfg.AgentWalletPrivateKey, s.cfg.DeploymentEscrowAddress, sessionID)
	}
	if err != nil {
		log.Printf("[api] dispute session=%s: %v", id, err)
		jsonError(w, fmt.Sprintf("dispute call failed: %v", err), http.StatusBadGateway)
		return
	}

	jsonOK(w, map[string]any{
		"status":       status,
		"session_id":   id,
		"to_provider":  req.ToProvider,
		"tx_hash":      txHash,
		"explorer_url": chain.MonadTxURL(txHash),
	})
}

// handleSlashProvider slashes the provider that ran a session. The evidence
// committed on-chain is the session's Merkle root, so anyone can replay the
// action log against it and check the claim.
func (s *Server) handleSlashProvider(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	if s.cfg.AgentWalletPrivateKey == "" || s.cfg.ProviderRegistryAddress == "" {
		jsonError(w, "slashing not configured on this node", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Provider string `json:"provider"` // optional override
		Reason   string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	providerAddr := req.Provider
	merkleRoot := ""
	if sess := s.lookupSession(id); sess != nil {
		merkleRoot = sess.MerkleRoot
		if providerAddr == "" && sess.SelectedProvider != nil {
			providerAddr = sess.SelectedProvider.Wallet.Hex()
		}
	}
	if merkleRoot == "" {
		if dbSess, err := s.db.GetSession(ctx, id); err == nil {
			merkleRoot = dbSess.MerkleRoot
		}
	}
	if providerAddr == "" {
		jsonError(w, "provider address required (session has no on-chain provider)", http.StatusBadRequest)
		return
	}

	// Evidence commitment: the session's Merkle root when we have one,
	// otherwise keccak256 over the stated reason.
	var evidence [32]byte
	if merkleRoot != "" {
		evidence, _ = chain.HexToBytes32(merkleRoot)
	} else {
		evidence = chain.SessionIDToJobID(id + "|" + req.Reason)
	}

	txHash, err := chain.SlashProvider(ctx, s.cfg.MonadTestnetRPCURL,
		s.cfg.AgentWalletPrivateKey, s.cfg.ProviderRegistryAddress,
		gethcommon.HexToAddress(providerAddr), evidence)
	if err != nil {
		log.Printf("[api] slash session=%s provider=%s: %v", id, providerAddr, err)
		jsonError(w, fmt.Sprintf("slash failed: %v", err), http.StatusBadGateway)
		return
	}

	jsonOK(w, map[string]any{
		"status":       "slashed",
		"session_id":   id,
		"provider":     providerAddr,
		"evidence":     merkleRoot,
		"tx_hash":      txHash,
		"explorer_url": chain.MonadTxURL(txHash),
	})
}

func (s *Server) handleGetAudit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var actions []agent.Action
	var merkleRoot string

	// Try in-memory session first (active or recently completed)
	if sess := s.lookupSession(id); sess != nil {
		actions = sess.Actions
		merkleRoot = sess.MerkleRoot
	} else {
		// Fall back to persisted action log in DB
		al, err := s.db.GetActionLog(r.Context(), id)
		if err != nil {
			jsonError(w, "session not found", http.StatusNotFound)
			return
		}
		if err := json.Unmarshal(al.Actions, &actions); err != nil {
			jsonError(w, "failed to decode action log", http.StatusInternalServerError)
			return
		}
		if dbSess, err := s.db.GetSession(r.Context(), id); err == nil {
			merkleRoot = dbSess.MerkleRoot
		}
	}

	type auditEntry struct {
		agent.Action
		Proof []string `json:"proof"`
	}
	entries := make([]auditEntry, len(actions))
	for i, a := range actions {
		entries[i] = auditEntry{Action: a, Proof: agent.ComputeMerkleProof(actions, i)}
	}

	jsonOK(w, map[string]any{
		"session_id":  id,
		"merkle_root": merkleRoot,
		"actions":     entries,
	})
}

func (s *Server) handleStreamSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess := s.lookupSession(id)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[api] ws upgrade: %v", err)
		return
	}
	defer conn.Close()

	// If session is not in-memory, try to replay stored events from 0G for completed sessions.
	if sess == nil {
		dbSess, dbErr := s.db.GetSession(r.Context(), id)
		if dbErr != nil || dbSess.State != "completed" {
			b, _ := json.Marshal(map[string]string{"type": "error", "error": "session not found"})
			_ = conn.WriteMessage(websocket.TextMessage, b)
			return
		}
		entries, readErr := s.zeroG.ReadLog(r.Context(), id)
		if readErr != nil {
			log.Printf("[api] 0G ReadLog for session %s: %v", id, readErr)
			// Still send a done event so the client knows the session completed
			b, _ := json.Marshal(map[string]string{"type": "done"})
			_ = conn.WriteMessage(websocket.TextMessage, b)
			return
		}
		for _, entry := range entries {
			if err := conn.WriteMessage(websocket.TextMessage, entry); err != nil {
				return
			}
		}
		// Signal end of replay
		b, _ := json.Marshal(map[string]string{"type": "done"})
		_ = conn.WriteMessage(websocket.TextMessage, b)
		return
	}

	events := sess.Events()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			b, _ := json.Marshal(event)
			if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}
			if event.Type == "done" || event.Type == "error" {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

// --- Providers ---

// handleScanRepo analyzes a repository and returns the runnable applications
// found in it, so /deploy can offer the user a framework to launch.
//
// Only a malformed request is an error here. If the scanner is unconfigured or
// the scan fails, it answers 200 with an empty options list: the UI treats that
// as "nothing detected" and falls back to a generic deployment prompt, and the
// agent analyzes the repo itself during the session anyway. A broken scan must
// not block a deployment.
func (s *Server) handleScanRepo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepoURL string `json:"repo_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	// A bad URL is the caller's mistake and is worth saying plainly; only
	// operational failures below fall back to an empty option list.
	repoURL, err := scanner.ValidateRepoURL(req.RepoURL)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	empty := func(note string) {
		jsonOK(w, map[string]any{
			"repo_url": repoURL,
			"options":  []scanner.DetectedOption{},
			"note":     note,
		})
	}

	if !s.sc.Configured() {
		log.Printf("[api] repo scan skipped: scanner not configured")
		empty("repository scanning is not configured on this node")
		return
	}

	plan, err := s.sc.AnalyzeRepo(r.Context(), repoURL)
	if err != nil {
		log.Printf("[api] repo scan %s: %v", repoURL, err)
		empty("could not analyze this repository automatically")
		return
	}

	options := plan.Options
	if options == nil {
		options = []scanner.DetectedOption{}
	}
	jsonOK(w, map[string]any{
		"repo_url": repoURL,
		"options":  options,
		"summary":  plan.Summary,
	})
}

func (s *Server) handleGetProviders(w http.ResponseWriter, r *http.Request) {
	type providerView struct {
		Wallet        string `json:"wallet"`
		Endpoint      string `json:"endpoint"`
		PricePerHour  string `json:"price_per_hour"`
		JobsCompleted uint64 `json:"jobs_completed"`
	}
	// Before the registry is deployed there simply are no providers. That is an
	// empty marketplace, not a server error, so answer with an empty list.
	if s.cfg.ProviderRegistryAddress == "" {
		jsonOK(w, []providerView{})
		return
	}
	providers, err := chain.GetActiveProviders(r.Context(), s.cfg.MonadTestnetRPCURL, s.cfg.ProviderRegistryAddress)
	if err != nil {
		log.Printf("[api] GetActiveProviders: %v", err)
		jsonOK(w, []providerView{})
		return
	}
	views := make([]providerView, len(providers))
	for i, p := range providers {
		views[i] = providerView{
			Wallet:        p.Wallet.Hex(),
			Endpoint:      p.Endpoint,
			PricePerHour:  p.PricePerHour.String(),
			JobsCompleted: p.JobsCompleted.Uint64(),
		}
	}
	jsonOK(w, views)
}

// handleSubmitBid lets a registered provider submit a bid for an open job auction.
// The provider must be authenticated (JWT). This node's configured
// PROVIDER_WALLET_PRIVATE_KEY is used to sign the on-chain submitBid() transaction.
func (s *Server) handleSubmitBid(w http.ResponseWriter, r *http.Request) {
	if s.cfg.JobAuctionAddress == "" {
		jsonError(w, "job auctions not configured on this node", http.StatusServiceUnavailable)
		return
	}
	if s.cfg.ProviderWalletPrivateKey == "" {
		jsonError(w, "provider wallet not configured on this node", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		SessionID    string `json:"session_id"`    // the session whose job to bid on
		PricePerHour string `json:"price_per_hour"` // wei per hour as decimal string
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionID == "" || req.PricePerHour == "" {
		jsonError(w, "session_id and price_per_hour required", http.StatusBadRequest)
		return
	}

	price, ok := new(big.Int).SetString(req.PricePerHour, 10)
	if !ok || price.Sign() <= 0 {
		jsonError(w, "invalid price_per_hour", http.StatusBadRequest)
		return
	}

	jobID := chain.SessionIDToJobID(req.SessionID)
	txHash, err := chain.SubmitBid(r.Context(),
		s.cfg.MonadTestnetRPCURL, s.cfg.ProviderWalletPrivateKey,
		s.cfg.JobAuctionAddress, jobID, price)
	if err != nil {
		log.Printf("[api] SubmitBid: %v", err)
		jsonError(w, fmt.Sprintf("submitBid failed: %v", err), http.StatusBadGateway)
		return
	}
	jsonOK(w, map[string]string{"tx_hash": txHash})
}

// StartProviderBidder launches a background goroutine that polls for JobPosted events
// and auto-submits bids when this node is running in provider mode.
// Call this once at server startup.
func (s *Server) StartProviderBidder(ctx context.Context) {
	if !s.cfg.ProviderMode || s.cfg.JobAuctionAddress == "" || s.cfg.ProviderWalletPrivateKey == "" {
		return
	}
	go func() {
		log.Printf("[bidder] provider mode active — watching %s for job auctions", s.cfg.JobAuctionAddress)

		// Start from the current chain head to avoid replaying old events.
		fromBlock, err := chain.CurrentBlock(ctx, s.cfg.MonadTestnetRPCURL)
		if err != nil {
			log.Printf("[bidder] could not get current block: %v — starting from 0", err)
			fromBlock = 0
		}

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				events, nextBlock, err := chain.PollJobPostedEvents(ctx, s.cfg.MonadTestnetRPCURL, s.cfg.JobAuctionAddress, fromBlock)
				if err != nil {
					log.Printf("[bidder] poll: %v", err)
					continue
				}
				fromBlock = nextBlock

				for _, ev := range events {
					// Skip if bid deadline has already passed.
					if ev.BidDeadline.Int64() > 0 && time.Now().Unix() > ev.BidDeadline.Int64() {
						continue
					}

					// Look up this provider's registered price, fallback to max ceiling.
					ourPrice := ev.MaxPricePerHour
					if s.cfg.ProviderRegistryAddress != "" {
						providers, err := chain.GetActiveProviders(ctx, s.cfg.MonadTestnetRPCURL, s.cfg.ProviderRegistryAddress)
						if err == nil {
							ourAddr := chain.AgentAddress(s.cfg.ProviderWalletPrivateKey)
							for _, p := range providers {
								if strings.EqualFold(p.Wallet.Hex(), ourAddr) {
									ourPrice = p.PricePerHour
									break
								}
							}
						}
					}

					// Don't bid above the job's ceiling.
					if ourPrice.Cmp(ev.MaxPricePerHour) > 0 {
						log.Printf("[bidder] job %x ceiling %s < our price %s — skipping", ev.JobID, ev.MaxPricePerHour, ourPrice)
						continue
					}

					txHash, err := chain.SubmitBid(ctx,
						s.cfg.MonadTestnetRPCURL, s.cfg.ProviderWalletPrivateKey,
						s.cfg.JobAuctionAddress, ev.JobID, ourPrice)
					if err != nil {
						log.Printf("[bidder] submitBid job %x: %v", ev.JobID, err)
					} else {
						log.Printf("[bidder] bid submitted for job %x at %s wei/hr (tx: %s)", ev.JobID, ourPrice, txHash)
					}
				}
			}
		}
	}()
}

// --- Workspaces ---

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	containers, err := s.db.ListTeamContainers(r.Context(), teamID)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, containers)
}

// --- Vault ---

func (s *Server) handleVaultNonce(w http.ResponseWriter, r *http.Request) {
	address := r.Context().Value(ctxKeyAddress).(string)
	nonce := s.authSvc.IssueNonce("vault:" + address)
	jsonOK(w, map[string]string{"nonce": nonce})
}

func (s *Server) handleVaultKey(w http.ResponseWriter, r *http.Request) {
	address := r.Context().Value(ctxKeyAddress).(string)
	var req struct {
		ContainerID string `json:"container_id"`
		Nonce       string `json:"nonce"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ContainerID == "" {
		jsonError(w, "container_id required", http.StatusBadRequest)
		return
	}
	if err := s.authSvc.ConsumeNonce("vault:"+address, req.Nonce); err != nil {
		jsonError(w, "invalid nonce", http.StatusUnauthorized)
		return
	}
	vaultKey := chain.DeriveVaultKey(s.cfg.VaultMasterSecret, req.ContainerID)
	jsonOK(w, map[string]string{"key": vaultKey})
}

// --- x402 middleware ---

// x402Middleware checks for a valid USDC transferWithAuthorization header.
// On missing or invalid payment it returns 402 with details.
func (s *Server) x402Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Local/demo nodes can turn the paywall off entirely. Guarded by an
		// explicit opt-in so a production node never does this by accident.
		if s.cfg.PaymentsDisabled {
			next(w, r)
			return
		}
		paymentHeader := r.Header.Get("X-Payment")
		if paymentHeader == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPaymentRequired)
			json.NewEncoder(w).Encode(map[string]any{
				"x402Version": 1,
				"error":       "payment_required",
				"accepts": []map[string]any{
					{
						"scheme":            "exact",
						"network":           "eip155:10143",
						"maxAmountRequired": "1000000", // 1 USDC (6 decimals)
						"resource":          r.URL.Path,
						"payTo":             chain.AgentAddress(s.cfg.AgentWalletPrivateKey),
						"maxTimeoutSeconds": 300,
						"asset":             chain.USDCAddress,
						"extra": map[string]string{
							"name":    "USD Coin",
							"version": "2",
						},
					},
				},
			})
			return
		}

		// Decode and verify the payment proof
		raw, err := base64.StdEncoding.DecodeString(paymentHeader)
		if err != nil {
			jsonError(w, "invalid X-Payment header encoding", http.StatusBadRequest)
			return
		}
		var auth chain.TransferAuth
		if err := json.Unmarshal(raw, &auth); err != nil {
			jsonError(w, "invalid X-Payment payload", http.StatusBadRequest)
			return
		}
		if err := chain.VerifyTransferAuth(auth); err != nil {
			jsonError(w, fmt.Sprintf("payment verification failed: %v", err), http.StatusPaymentRequired)
			return
		}

		// Idempotency — prevent replay
		ctx := r.Context()
		if exists, _ := s.db.PaymentNonceExists(ctx, fmt.Sprintf("%x", auth.Nonce)); exists {
			jsonError(w, "payment already used", http.StatusPaymentRequired)
			return
		}
		if err := s.db.CreatePayment(ctx, &store.Payment{
			Nonce:      fmt.Sprintf("%x", auth.Nonce),
			Wallet:     auth.From.Hex(),
			AmountUSDC: auth.Value.String(),
			Status:     "pending",
		}); err != nil {
			log.Printf("[api] RecordPayment: %v", err)
		}

		// Execute on-chain in background (fire-and-forget for demo; real impl should wait)
		go func() {
			txHash, err := chain.ExecuteTransferAuth(
				context.Background(), s.cfg.MonadTestnetRPCURL,
				s.cfg.AgentWalletPrivateKey, auth)
			if err != nil {
				log.Printf("[api] ExecuteTransferAuth: %v", err)
			} else {
				log.Printf("[api] USDC transfer tx: %s", txHash)
			}
		}()

		next.ServeHTTP(w, r)
	}
}

// --- JWT middleware ---

type contextKey string

const (
	ctxKeyAddress contextKey = "address"
	ctxKeyTeamID  contextKey = "team_id"
)

func (s *Server) jwtMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			jsonError(w, "authorization required", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		address, teamID, err := s.authSvc.ValidateJWT(tokenStr)
		if err != nil {
			jsonError(w, "invalid token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyAddress, address)
		ctx = context.WithValue(ctx, ctxKeyTeamID, teamID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// wsAuthMiddleware authenticates routes a browser opens over WebSocket.
//
// The browser WebSocket API cannot set request headers, so a JWT can never
// arrive as "Authorization: Bearer" on those connections. This middleware
// accepts the same token as a ?token= query parameter instead, and is applied
// only to the stream endpoint. Query strings are easier to leak into access
// logs than headers, so do not reuse this for ordinary HTTP routes.
func (s *Server) wsAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tokenStr == r.Header.Get("Authorization") {
			// no Bearer prefix was present; fall back to the query parameter
			tokenStr = r.URL.Query().Get("token")
		}
		if tokenStr == "" {
			jsonError(w, "authorization required", http.StatusUnauthorized)
			return
		}
		address, teamID, err := s.authSvc.ValidateJWT(tokenStr)
		if err != nil {
			jsonError(w, "invalid token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyAddress, address)
		ctx = context.WithValue(ctx, ctxKeyTeamID, teamID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// --- CORS middleware ---

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Payment")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Subdomain proxy middleware ---

// subdomainProxyMiddleware intercepts requests whose Host header matches
// <containerID>.<DEPLOY_DOMAIN> and reverse-proxies them to the container's
// mapped port on localhost. All other requests fall through to next.
func (s *Server) subdomainProxyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.DeployDomain == "" {
			next.ServeHTTP(w, r)
			return
		}
		host := r.Host
		// Strip port if present
		if idx := strings.LastIndex(host, ":"); idx > 0 {
			host = host[:idx]
		}
		suffix := "." + s.cfg.DeployDomain
		if !strings.HasSuffix(host, suffix) {
			next.ServeHTTP(w, r)
			return
		}
		containerID := strings.TrimSuffix(host, suffix)
		if containerID == "" {
			next.ServeHTTP(w, r)
			return
		}
		hostPort, ok := s.mgr.LookupDeployPort(containerID)
		if !ok {
			http.Error(w, "container not found or not running", http.StatusBadGateway)
			return
		}
		target, _ := url.Parse("http://127.0.0.1:" + hostPort)
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[proxy] %s → %s: %v", host, target, err)
			http.Error(w, "container unreachable", http.StatusBadGateway)
		}
		// Rewrite Host header so the container sees its own origin
		r2 := r.Clone(r.Context())
		r2.Host = target.Host
		proxy.ServeHTTP(w, r2)
	})
}

// --- Account ---

// handleGetAccount returns the team record for a wallet address (no JWT required).
// Used by AuthContext.fetchAccount on every page load.
func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	wallet := r.URL.Query().Get("wallet")
	if wallet == "" {
		jsonError(w, "wallet query param required", http.StatusBadRequest)
		return
	}
	team, err := s.db.GetOrCreateTeamByWallet(r.Context(), strings.ToLower(wallet))
	if err != nil {
		log.Printf("[api] GetOrCreateTeamByWallet: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{
		"id":     team.ID,
		"name":   team.Name,
		"wallet": team.Wallet,
	})
}

// handleUpdateAccount updates the team display name (used by the onboarding flow).
func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}
	if err := s.db.UpdateTeamName(r.Context(), teamID, req.Name); err != nil {
		log.Printf("[api] UpdateTeamName: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"team_id": teamID, "team_name": req.Name})
}

// --- Team resources ---

func (s *Server) handleListTeamSessions(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	sessions, err := s.db.ListTeamSessions(r.Context(), teamID)
	if err != nil {
		log.Printf("[api] ListTeamSessions: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if sessions == nil {
		sessions = []store.Session{}
	}
	jsonOK(w, sessions)
}

func (s *Server) handleGetAttestation(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionId")
	a, err := s.db.GetAttestation(r.Context(), sessionID)
	if err != nil {
		jsonError(w, "attestation not found", http.StatusNotFound)
		return
	}
	jsonOK(w, a)
}

func (s *Server) handleListTeamAttestations(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	attestations, err := s.db.ListTeamAttestations(r.Context(), teamID)
	if err != nil {
		log.Printf("[api] ListTeamAttestations: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if attestations == nil {
		attestations = []store.Attestation{}
	}
	jsonOK(w, attestations)
}

// --- Payments ---

func (s *Server) handleListPayments(w http.ResponseWriter, r *http.Request) {
	address := r.Context().Value(ctxKeyAddress).(string)
	payments, err := s.db.ListPaymentsByWallet(r.Context(), address)
	if err != nil {
		log.Printf("[api] ListPaymentsByWallet: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if payments == nil {
		payments = []store.Payment{}
	}
	jsonOK(w, payments)
}

// --- Secrets ---

func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	secrets, err := s.db.ListSecrets(r.Context(), teamID)
	if err != nil {
		log.Printf("[api] ListSecrets: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if secrets == nil {
		secrets = []store.Secret{}
	}
	jsonOK(w, secrets)
}

func (s *Server) handleCreateSecret(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	var req struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Value == "" {
		jsonError(w, "name and value required", http.StatusBadRequest)
		return
	}
	name := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(req.Name), " ", "_"))
	encrypted, err := encryptSecret(s.cfg.VaultMasterSecret, req.Value)
	if err != nil {
		log.Printf("[api] encryptSecret: %v", err)
		jsonError(w, "encryption error", http.StatusInternalServerError)
		return
	}
	sec, err := s.db.CreateSecret(r.Context(), teamID, name, encrypted)
	if err != nil {
		log.Printf("[api] CreateSecret: %v", err)
		jsonError(w, "could not create secret (name may already exist)", http.StatusBadRequest)
		return
	}
	jsonOK(w, sec)
}

func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	idStr := chi.URLParam(r, "id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id <= 0 {
		jsonError(w, "invalid secret id", http.StatusBadRequest)
		return
	}
	if err := s.db.DeleteSecret(r.Context(), id, teamID); err != nil {
		log.Printf("[api] DeleteSecret: %v", err)
		jsonError(w, "could not delete secret", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Projects ---

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	projects, err := s.db.ListProjects(r.Context(), teamID)
	if err != nil {
		log.Printf("[api] ListProjects: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if projects == nil {
		projects = []store.Project{}
	}
	jsonOK(w, projects)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	var req struct {
		Name       string `json:"name"`
		RepoURL    string `json:"repo_url"`
		Branch     string `json:"branch"`
		AutoDeploy bool   `json:"auto_deploy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}
	branch := req.Branch
	if branch == "" {
		branch = "main"
	}
	// Generate a random webhook secret for this project
	webhookBytes := make([]byte, 24)
	_, _ = rand.Read(webhookBytes)
	webhookSecret := base64.RawURLEncoding.EncodeToString(webhookBytes)

	proj := &store.Project{
		ID:            newUUID(),
		TeamID:        teamID,
		Name:          req.Name,
		RepoURL:       req.RepoURL,
		Branch:        branch,
		WebhookSecret: webhookSecret,
		AutoDeploy:    req.AutoDeploy,
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.db.CreateProject(r.Context(), proj); err != nil {
		log.Printf("[api] CreateProject: %v", err)
		jsonError(w, "could not create project", http.StatusInternalServerError)
		return
	}
	// Return the webhook secret only on creation (never again)
	jsonOK(w, proj)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	id := chi.URLParam(r, "id")
	proj, err := s.db.GetProject(r.Context(), id, teamID)
	if err != nil {
		jsonError(w, "project not found", http.StatusNotFound)
		return
	}
	proj.WebhookSecret = "" // redact after initial creation
	jsonOK(w, proj)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	id := chi.URLParam(r, "id")
	var req struct {
		Name       string `json:"name"`
		RepoURL    string `json:"repo_url"`
		Branch     string `json:"branch"`
		LastPrompt string `json:"last_prompt"`
		AutoDeploy bool   `json:"auto_deploy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}
	if err := s.db.UpdateProject(r.Context(), id, teamID, req.Name, req.RepoURL, req.Branch, req.LastPrompt, req.AutoDeploy); err != nil {
		jsonError(w, "could not update project", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	id := chi.URLParam(r, "id")
	if err := s.db.DeleteProject(r.Context(), id, teamID); err != nil {
		jsonError(w, "could not delete project", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Project Env Vars ---

func (s *Server) handleListProjectEnv(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	projectID := chi.URLParam(r, "id")
	vars, err := s.db.ListProjectEnvVars(r.Context(), projectID, teamID)
	if err != nil {
		log.Printf("[api] ListProjectEnvVars: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if vars == nil {
		vars = []store.ProjectEnvVar{}
	}
	jsonOK(w, vars)
}

func (s *Server) handleUpsertProjectEnv(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	projectID := chi.URLParam(r, "id")
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		jsonError(w, "key required", http.StatusBadRequest)
		return
	}
	// Validate project belongs to team
	if _, err := s.db.GetProject(r.Context(), projectID, teamID); err != nil {
		jsonError(w, "project not found", http.StatusNotFound)
		return
	}
	key := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(req.Key), " ", "_"))
	encrypted, err := encryptSecret(s.cfg.VaultMasterSecret, req.Value)
	if err != nil {
		jsonError(w, "encryption error", http.StatusInternalServerError)
		return
	}
	v, err := s.db.UpsertProjectEnvVar(r.Context(), projectID, teamID, key, encrypted)
	if err != nil {
		log.Printf("[api] UpsertProjectEnvVar: %v", err)
		jsonError(w, "could not save env var", http.StatusInternalServerError)
		return
	}
	jsonOK(w, v)
}

func (s *Server) handleDeleteProjectEnv(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	projectID := chi.URLParam(r, "id")
	idStr := chi.URLParam(r, "envId")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id <= 0 {
		jsonError(w, "invalid env var id", http.StatusBadRequest)
		return
	}
	if err := s.db.DeleteProjectEnvVar(r.Context(), id, projectID, teamID); err != nil {
		jsonError(w, "could not delete env var", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- GitHub Webhook (CI/CD) ---

// handleGitHubWebhook receives a GitHub push event and triggers a redeploy.
// Authentication: HMAC-SHA256 of the raw request body with the project's webhook_secret,
// sent by GitHub in the X-Hub-Signature-256 header.
func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectId")

	// Read raw body for HMAC verification
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		jsonError(w, "body read error", http.StatusBadRequest)
		return
	}

	proj, err := s.db.GetProjectByWebhook(r.Context(), projectID)
	if err != nil {
		// Return 200 to avoid leaking project existence to probers
		w.WriteHeader(http.StatusOK)
		return
	}

	// Verify GitHub HMAC signature
	sig := r.Header.Get("X-Hub-Signature-256")
	if !verifyGitHubHMAC(proj.WebhookSecret, body, sig) {
		jsonError(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	// Only trigger on push events
	if r.Header.Get("X-GitHub-Event") != "push" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Parse push event to check branch
	var pushEvent struct {
		Ref string `json:"ref"` // e.g. "refs/heads/main"
	}
	_ = json.Unmarshal(body, &pushEvent)
	expectedRef := "refs/heads/" + proj.Branch
	if pushEvent.Ref != "" && pushEvent.Ref != expectedRef {
		// Push to a different branch — ignore
		w.WriteHeader(http.StatusOK)
		return
	}

	if !proj.AutoDeploy || proj.LastPrompt == "" {
		jsonOK(w, map[string]string{"status": "skipped", "reason": "auto_deploy disabled or no previous deploy"})
		return
	}

	// Load + decrypt project env vars
	encVars, _ := s.db.GetProjectEnvVarValues(r.Context(), projectID, proj.TeamID)
	mergedEnv := map[string]string{}
	for k, enc := range encVars {
		if plain, err := decryptSecret(s.cfg.VaultMasterSecret, enc); err == nil {
			mergedEnv[k] = plain
		}
	}

	// Create a new session using the last deploy prompt
	sessionID := newUUID()
	if err := s.db.CreateSession(r.Context(), &store.Session{
		ID:     sessionID,
		TeamID: proj.TeamID,
		Prompt: proj.LastPrompt,
		State:  "running",
	}); err != nil {
		log.Printf("[webhook] CreateSession: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	sess := agent.NewSession(s.sessionConfig(sessionID, proj.TeamID, mergedEnv, true))
	sess.Finalizer = s.chainFinalizer(sessionID, proj.TeamID)
	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
		defer cancel()
		_ = sess.Run(ctx, proj.LastPrompt)
		_ = s.db.UpsertActionLog(ctx, sessionID, proj.TeamID, sess.Actions)
		_ = s.db.UpdateSessionState(ctx, sessionID, string(sess.State))
		_ = s.db.UpdateSessionMerkleRoot(ctx, sessionID, sess.MerkleRoot, "")
		_ = s.db.TouchProjectDeployedAt(ctx, projectID)
	}()

	jsonOK(w, map[string]string{"status": "triggered", "session_id": sessionID})
}


// verifyGitHubHMAC checks the X-Hub-Signature-256 header from GitHub.
// GitHub sends "sha256=<hex(HMAC-SHA256(secret, body))>".
func verifyGitHubHMAC(secret string, body []byte, sigHeader string) bool {
	if !strings.HasPrefix(sigHeader, "sha256=") {
		return false
	}
	expected := strings.TrimPrefix(sigHeader, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	actual := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(actual))
}

// --- helpers ---

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *Server) lookupSession(id string) *agent.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[id]
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
