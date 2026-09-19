package chain

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Provider scoring. The marketplace does not simply pick the cheapest node:
// each active provider from the on-chain registry is scored on price,
// reputation (completed jobs), staked collateral and slash history. The score
// is what the agent uses to justify its pick during the demo, and what the
// /providers/scored endpoint surfaces to the UI.

// Scoring weights. They sum to 1.0.
const (
	WeightPrice       = 0.45
	WeightReliability = 0.30
	WeightStake       = 0.25

	// SlashPenalty is deducted from the final score for every recorded slash.
	SlashPenalty = 25.0

	// ReliabilityHalfLife is the number of completed jobs at which a provider
	// scores 50/100 on reliability. The curve saturates, so an established
	// provider cannot outrank everyone on job count alone.
	ReliabilityHalfLife = 5.0
)

// ScoredProvider is a registry provider annotated with its marketplace score.
// Wei amounts are serialised as strings: they exceed JavaScript's safe integer
// range, so a JSON number would silently lose precision in the browser.
type ScoredProvider struct {
	Provider `json:"-"`

	Endpoint         string  `json:"endpoint"`
	PricePerHourWei  string  `json:"price_per_hour_wei"`
	PricePerHourMON  string  `json:"price_per_hour_mon"`
	StakedWei        string  `json:"staked_wei"`
	StakedMON        string  `json:"staked_mon"`
	JobsCompleted    string  `json:"jobs_completed"`
	SlashCount       string  `json:"slash_count"`
	Active           bool    `json:"active"`

	Score            float64 `json:"score"`
	PriceScore       float64 `json:"price_score"`
	ReliabilityScore float64 `json:"reliability_score"`
	StakeScore       float64 `json:"stake_score"`
	SlashPenalty     float64 `json:"slash_penalty"`

	// LatencyMs is the measured /health round-trip. -1 means unreachable or
	// not probed.
	LatencyMs int64 `json:"latency_ms"`

	// Reason is a human-readable justification used in the demo stream.
	Reason string `json:"reason"`

	// Rank is 1-based, assigned after sorting.
	Rank int `json:"rank"`

	// WalletHex / explorer link, convenient for JSON consumers.
	WalletHex  string `json:"wallet"`
	ExplorerURL string `json:"explorer_url"`
}

// ScoreProviders ranks providers best-first. It never returns an error: an
// empty input yields an empty slice.
func ScoreProviders(providers []Provider) []ScoredProvider {
	out := make([]ScoredProvider, 0, len(providers))
	if len(providers) == 0 {
		return out
	}

	maxPrice := big.NewInt(0)
	maxStake := big.NewInt(0)
	for _, p := range providers {
		if p.PricePerHour != nil && p.PricePerHour.Cmp(maxPrice) > 0 {
			maxPrice = p.PricePerHour
		}
		if p.StakedAmount != nil && p.StakedAmount.Cmp(maxStake) > 0 {
			maxStake = p.StakedAmount
		}
	}

	for _, p := range providers {
		sp := ScoredProvider{
			Provider:        p,
			Endpoint:        p.Endpoint,
			PricePerHourWei: bigString(p.PricePerHour),
			PricePerHourMON: FormatMON(p.PricePerHour),
			StakedWei:       bigString(p.StakedAmount),
			StakedMON:       FormatMON(p.StakedAmount),
			JobsCompleted:   bigString(p.JobsCompleted),
			SlashCount:      bigString(p.SlashCount),
			Active:          p.Active,
			LatencyMs:       -1,
			WalletHex:       p.Wallet.Hex(),
			ExplorerURL:     MonadAddressURL(p.Wallet.Hex()),
		}

		// Price: cheapest in the set scores 100, most expensive scores 0.
		// A single-provider set, or an all-free set, scores 100.
		sp.PriceScore = 100
		if maxPrice.Sign() > 0 && p.PricePerHour != nil {
			sp.PriceScore = 100 * (1 - ratio(p.PricePerHour, maxPrice))
		}

		// Reliability: saturating curve over completed jobs.
		jobs := bigToFloat(p.JobsCompleted)
		sp.ReliabilityScore = 100 * (jobs / (jobs + ReliabilityHalfLife))

		// Stake: skin in the game, normalised against the largest stake.
		sp.StakeScore = 0
		if maxStake.Sign() > 0 && p.StakedAmount != nil {
			sp.StakeScore = 100 * ratio(p.StakedAmount, maxStake)
		}

		sp.SlashPenalty = SlashPenalty * bigToFloat(p.SlashCount)

		sp.Score = WeightPrice*sp.PriceScore +
			WeightReliability*sp.ReliabilityScore +
			WeightStake*sp.StakeScore -
			sp.SlashPenalty
		if sp.Score < 0 {
			sp.Score = 0
		}

		sp.Reason = fmt.Sprintf(
			"price %.0f/100 · reliability %.0f/100 (%s jobs) · stake %.0f/100 · %s slash(es)",
			sp.PriceScore, sp.ReliabilityScore, bigString(p.JobsCompleted),
			sp.StakeScore, bigString(p.SlashCount),
		)

		out = append(out, sp)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		// Tie-break on price, then on completed jobs.
		if c := cmpBig(out[i].Provider.PricePerHour, out[j].Provider.PricePerHour); c != 0 {
			return c < 0
		}
		return cmpBig(out[i].Provider.JobsCompleted, out[j].Provider.JobsCompleted) > 0
	})

	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

// ProbeLatency health-checks every provider endpoint in parallel and records
// the round-trip in LatencyMs. Unreachable providers keep LatencyMs = -1 and
// are pushed to the back of the ranking, since an offline node cannot run a
// workload no matter how cheap it is.
func ProbeLatency(ctx context.Context, providers []ScoredProvider, timeout time.Duration) []ScoredProvider {
	if len(providers) == 0 {
		return providers
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	var wg sync.WaitGroup
	for i := range providers {
		if providers[i].Endpoint == "" {
			continue
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			providers[idx].LatencyMs = probeOne(ctx, providers[idx].Endpoint, timeout)
		}(i)
	}
	wg.Wait()

	// Re-rank: reachable providers first, then by score.
	sort.SliceStable(providers, func(i, j int) bool {
		iUp := providers[i].LatencyMs >= 0
		jUp := providers[j].LatencyMs >= 0
		if iUp != jUp {
			return iUp
		}
		return providers[i].Score > providers[j].Score
	})
	for i := range providers {
		providers[i].Rank = i + 1
		if providers[i].LatencyMs < 0 {
			providers[i].Reason += " · endpoint unreachable"
		}
	}
	return providers
}

func probeOne(ctx context.Context, endpoint string, timeout time.Duration) int64 {
	url := trimSlash(endpoint) + "/health"
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(pingCtx, http.MethodGet, url, nil)
	if err != nil {
		return -1
	}
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return -1
	}
	return time.Since(start).Milliseconds()
}

// SelectBestProvider scores the active registry providers and returns the
// highest-ranked reachable one, along with the full ranking so the caller can
// show what was compared.
func SelectBestProvider(ctx context.Context, rpcURL, registryAddress string) (*ScoredProvider, []ScoredProvider, error) {
	providers, err := GetActiveProviders(ctx, rpcURL, registryAddress)
	if err != nil {
		return nil, nil, err
	}
	scored := ScoreProviders(providers)
	if len(scored) == 0 {
		return nil, scored, nil
	}
	scored = ProbeLatency(ctx, scored, 3*time.Second)
	best := scored[0]
	if best.LatencyMs < 0 {
		// Every provider is unreachable — let the caller fall back.
		return nil, scored, nil
	}
	return &best, scored, nil
}

// --- helpers ---

func ratio(a, b *big.Int) float64 {
	if a == nil || b == nil || b.Sign() == 0 {
		return 0
	}
	af := new(big.Float).SetInt(a)
	bf := new(big.Float).SetInt(b)
	r, _ := new(big.Float).Quo(af, bf).Float64()
	return r
}

func bigToFloat(v *big.Int) float64 {
	if v == nil {
		return 0
	}
	f, _ := new(big.Float).SetInt(v).Float64()
	return f
}

func bigString(v *big.Int) string {
	if v == nil {
		return "0"
	}
	return v.String()
}

func cmpBig(a, b *big.Int) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	return a.Cmp(b)
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
