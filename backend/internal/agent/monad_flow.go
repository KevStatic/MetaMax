package agent

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/shreejaykurhade/MetaMax/backend/internal/chain"
	gethcommon "github.com/ethereum/go-ethereum/common"
)

// The Monad-facing half of a session: pipeline stage events, marketplace
// provider scoring, and the agent wallet's own escrow transaction. Everything
// here is what the "Deploy my AI workload" demo renders, one row per stage,
// each on-chain row carrying a MonadScan link.

// StageEnter announces that the agent has started a pipeline stage.
func (s *Session) StageEnter(stage, detail string) {
	s.stageMu.Lock()
	if s.stagesSeen == nil {
		s.stagesSeen = map[string]bool{}
	}
	s.stagesSeen[stage] = true
	s.stageMu.Unlock()

	s.emit(Event{Type: "stage", Stage: stage, Status: "active", Detail: detail})
}

// StageDone marks a stage complete, optionally with a Monad transaction.
// Passing an empty txHash is fine for off-chain stages.
func (s *Session) StageDone(stage, detail, txHash string) {
	s.emit(Event{
		Type:        "stage",
		Stage:       stage,
		Status:      "done",
		Detail:      detail,
		TxHash:      txHash,
		ExplorerURL: chain.MonadTxURL(txHash),
	})
}

// StageData marks a stage complete and attaches structured data (used by the
// comparison stage to ship the full provider ranking to the UI).
func (s *Session) StageData(stage, detail string, data any) {
	s.emit(Event{Type: "stage", Stage: stage, Status: "done", Detail: detail, Data: data})
}

// StageFailed marks a stage as failed but non-fatal; the pipeline keeps going.
func (s *Session) StageFailed(stage, detail string) {
	s.emit(Event{Type: "stage", Stage: stage, Status: "failed", Detail: detail})
}

// StageSkipped marks a stage the node is not configured for (e.g. no agent
// wallet key, so no escrow). The UI greys it out instead of showing a failure.
func (s *Session) StageSkipped(stage, detail string) {
	s.emit(Event{Type: "stage", Stage: stage, Status: "skipped", Detail: detail})
}

// chainReady reports whether this node can send Monad transactions.
func (s *Session) chainReady() bool {
	return s.agentPrivKey != "" && s.rpcURL != ""
}

// selectProviderScored is the marketplace's AI provider selection. It reads
// every active provider from the on-chain ProviderRegistry, scores them on
// price, reputation, stake and slash history, health-checks their endpoints in
// parallel, and picks the best one — narrating each step as a pipeline stage.
func (s *Session) selectProviderScored(ctx context.Context) (any, error) {
	s.StageEnter(StageFindingProviders, "Reading ProviderRegistry on Monad Testnet")

	providers, err := chain.GetActiveProviders(ctx, s.rpcURL, s.registryAddress)
	if err != nil {
		s.StageFailed(StageFindingProviders, fmt.Sprintf("registry query failed: %v", err))
		return s.localFallbackProvider("registry unreachable"), nil
	}
	if len(providers) == 0 {
		s.StageFailed(StageFindingProviders, "no active providers registered")
		return s.localFallbackProvider("no active providers"), nil
	}
	s.StageDone(StageFindingProviders, fmt.Sprintf("Found %d staked provider(s) on-chain", len(providers)), "")

	s.StageEnter(StageComparing, fmt.Sprintf("Scoring %d node(s) on price, reputation, stake and latency", len(providers)))
	ranking := chain.ScoreProviders(providers)
	ranking = chain.ProbeLatency(ctx, ranking, 3*time.Second)
	s.ProviderRanking = ranking
	s.StageData(StageComparing, fmt.Sprintf("Compared %d nodes", len(ranking)), ranking)

	// First reachable provider in the ranking wins.
	var winner *chain.ScoredProvider
	for i := range ranking {
		if ranking[i].LatencyMs >= 0 {
			winner = &ranking[i]
			break
		}
	}
	if winner == nil {
		s.StageFailed(StageSelecting, "every registered provider failed its health check")
		return s.localFallbackProvider("all providers unreachable"), nil
	}

	s.StageEnter(StageSelecting, fmt.Sprintf("Selecting Provider #%d", winner.Rank))
	provider := winner.Provider
	s.SelectedProvider = &provider
	s.emit(Event{
		Type:        "stage",
		Stage:       StageSelecting,
		Status:      "done",
		Detail:      fmt.Sprintf("Provider #%d %s — score %.1f/100, %s MON/hr, %d ms", winner.Rank, shortAddr(winner.WalletHex), winner.Score, winner.PricePerHourMON, winner.LatencyMs),
		ExplorerURL: winner.ExplorerURL,
		Data:        winner,
	})

	return map[string]any{
		"wallet":         winner.WalletHex,
		"endpoint":       winner.Endpoint,
		"price_per_hour": winner.PricePerHourWei,
		"jobs_completed": winner.JobsCompleted,
		"staked_amount":  winner.StakedWei,
		"slash_count":    winner.SlashCount,
		"score":          winner.Score,
		"rank":           winner.Rank,
		"latency_ms":     winner.LatencyMs,
		"reason":         winner.Reason,
		"explorer_url":   winner.ExplorerURL,
		"compared":       len(ranking),
		"source":         "on-chain-scored",
	}, nil
}

func (s *Session) localFallbackProvider(reason string) map[string]any {
	s.StageSkipped(StageComparing, reason)
	s.StageSkipped(StageSelecting, "falling back to the local MetaMax node")
	return map[string]any{
		"endpoint":       "http://localhost:8081",
		"price_per_hour": "0",
		"source":         "fallback",
		"reason":         reason,
	}
}

// fundEscrow locks native MON for the selected provider using the agent's own
// wallet. This is the agent-to-provider transaction: no human signs it, and
// the provider cannot be paid from anywhere else.
func (s *Session) fundEscrow(ctx context.Context) {
	if s.SelectedProvider == nil || s.SelectedProvider.Wallet == (zeroAddress) {
		s.StageSkipped(StageEscrow, "no on-chain provider selected — nothing to escrow")
		return
	}
	if !s.chainReady() || s.escrowAddress == "" || s.escrowDepositMON == "" {
		s.StageSkipped(StageEscrow, "escrow not configured on this node")
		return
	}

	deposit, err := chain.MONToWei(s.escrowDepositMON)
	if err != nil || deposit.Sign() <= 0 {
		s.StageSkipped(StageEscrow, "invalid escrow deposit amount")
		return
	}

	s.StageEnter(StageEscrow, fmt.Sprintf("Locking %s MON for %s", s.escrowDepositMON, shortAddr(s.SelectedProvider.Wallet.Hex())))

	jobID := chain.SessionIDToJobID(s.ID)
	txHash, err := chain.DepositEscrow(ctx, s.rpcURL, s.agentPrivKey, s.escrowAddress,
		jobID, s.SelectedProvider.Wallet, deposit)
	if err != nil {
		log.Printf("[session %s] escrow deposit: %v", s.ID, err)
		s.StageFailed(StageEscrow, fmt.Sprintf("escrow deposit failed: %v", err))
		return
	}

	s.EscrowTxHash = txHash
	s.StageDone(StageEscrow, fmt.Sprintf("%s MON escrowed for the provider", s.escrowDepositMON), txHash)
}

// SettleAndAttest is the tail of the pipeline: commit the execution proof to
// Monad, bump the provider's reputation, and release the escrowed payment.
// It runs as the session Finalizer so its transactions stream to the UI before
// the session closes.
func (s *Session) SettleAndAttest(ctx context.Context) {
	// --- Attest the Merkle root on Monad ---
	if !s.chainReady() || s.attestationAddress == "" || s.MerkleRoot == "" {
		s.StageSkipped(StageAttesting, "attestation contract not configured on this node")
	} else {
		s.StageEnter(StageAttesting, "Committing the execution Merkle root to ExecutionAttestation")
		merkleArr, convErr := chain.HexToBytes32(s.MerkleRoot)
		if convErr != nil {
			s.StageFailed(StageAttesting, fmt.Sprintf("invalid merkle root: %v", convErr))
		} else if result, err := chain.SubmitMonadAttestation(ctx, s.rpcURL, s.agentPrivKey,
			s.attestationAddress, s.ID, s.TeamID, merkleArr); err != nil {
			log.Printf("[session %s] attestation: %v", s.ID, err)
			s.StageFailed(StageAttesting, fmt.Sprintf("attestation failed: %v", err))
		} else {
			s.AttestTxHash = result.TxHash
			s.StageDone(StageAttesting, "Execution proof attested on Monad", result.TxHash)
		}
	}

	// --- Reputation + payment settlement ---
	if s.SelectedProvider == nil || s.SelectedProvider.Wallet == zeroAddress || !s.chainReady() {
		s.StageSkipped(StageSettlement, "no on-chain provider to pay")
		return
	}

	s.StageEnter(StageSettlement, fmt.Sprintf("Releasing escrow to %s", shortAddr(s.SelectedProvider.Wallet.Hex())))

	// Reputation: recordJobCompleted increments the provider's on-chain counter,
	// which feeds directly back into the next selection's score.
	if s.registryAddress != "" {
		if txHash, err := chain.RecordJobCompleted(ctx, s.rpcURL, s.agentPrivKey,
			s.registryAddress, s.SelectedProvider.Wallet); err != nil {
			log.Printf("[session %s] recordJobCompleted: %v", s.ID, err)
		} else {
			s.ReputationTxHash = txHash
			s.emit(Event{
				Type: "stage", Stage: StageSettlement, Status: "active",
				Detail:      "Provider reputation incremented on-chain",
				TxHash:      txHash,
				ExplorerURL: chain.MonadTxURL(txHash),
			})
		}
	}

	if s.escrowAddress == "" {
		s.StageSkipped(StageSettlement, "escrow contract not configured")
		return
	}

	jobID := chain.SessionIDToJobID(s.ID)
	txHash, err := chain.ReleaseEscrow(ctx, s.rpcURL, s.agentPrivKey, s.escrowAddress, jobID)
	if err != nil {
		// Expected when no escrow was ever funded for this session.
		log.Printf("[session %s] release escrow (non-fatal): %v", s.ID, err)
		s.StageFailed(StageSettlement, fmt.Sprintf("release failed: %v", err))
		return
	}
	s.SettlementTxHash = txHash
	s.StageDone(StageSettlement, "Payment released to the provider", txHash)
}

// ChainSummary is the on-chain receipt for a finished session, returned by the
// API so the UI can show every Monad transaction the run produced.
type ChainSummary struct {
	SessionID        string `json:"session_id"`
	ChainID          int64  `json:"chain_id"`
	Provider         string `json:"provider,omitempty"`
	ProviderExplorer string `json:"provider_explorer_url,omitempty"`
	MerkleRoot       string `json:"merkle_root,omitempty"`

	EscrowTx     string `json:"escrow_tx,omitempty"`
	EscrowURL    string `json:"escrow_url,omitempty"`
	AttestTx     string `json:"attest_tx,omitempty"`
	AttestURL    string `json:"attest_url,omitempty"`
	SettleTx     string `json:"settle_tx,omitempty"`
	SettleURL    string `json:"settle_url,omitempty"`
	ReputationTx string `json:"reputation_tx,omitempty"`
	ReputationURL string `json:"reputation_url,omitempty"`
}

// ChainSummary collects every Monad transaction this session produced.
func (s *Session) ChainSummary() ChainSummary {
	sum := ChainSummary{
		SessionID:     s.ID,
		ChainID:       chain.MonadTestnetChainID,
		MerkleRoot:    s.MerkleRoot,
		EscrowTx:      s.EscrowTxHash,
		EscrowURL:     chain.MonadTxURL(s.EscrowTxHash),
		AttestTx:      s.AttestTxHash,
		AttestURL:     chain.MonadTxURL(s.AttestTxHash),
		SettleTx:      s.SettlementTxHash,
		SettleURL:     chain.MonadTxURL(s.SettlementTxHash),
		ReputationTx:  s.ReputationTxHash,
		ReputationURL: chain.MonadTxURL(s.ReputationTxHash),
	}
	if s.SelectedProvider != nil && s.SelectedProvider.Wallet != zeroAddress {
		sum.Provider = s.SelectedProvider.Wallet.Hex()
		sum.ProviderExplorer = chain.MonadAddressURL(sum.Provider)
	}
	return sum
}

// --- small helpers ---

var zeroAddress gethcommon.Address

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func shortAddr(a string) string {
	if len(a) < 12 {
		return a
	}
	return a[:6] + "…" + a[len(a)-4:]
}
