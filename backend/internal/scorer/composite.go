package scorer

import (
	"encoding/json"
	"sort"

	"github.com/xmaeltht/trading-watchlist/internal/store"
)

// HorizonDays maps horizon to approximate holding window for risk calculations.
var HorizonDays = map[store.Horizon]int{
	store.HorizonDaily:   3,
	store.HorizonWeekly:  10,
	store.HorizonMonthly: 45,
}

// ScoreAll scores a slice of ticker inputs and returns ranked results for a given horizon.
// It enforces:
//   - Sector concentration limit (maxPerSector)
//   - Score normalization
//   - Confidence calculation
func ScoreAll(inputs []TickerInput, horizon store.Horizon, maxPerSector int) []ScoreResult {
	w := Weights[horizon]
	days := HorizonDays[horizon]

	raw := make([]ScoreResult, 0, len(inputs))

	for i := range inputs {
		t := &inputs[i]
		momentum := ScoreMomentum(t)
		volatility := ScoreVolatility(t)
		liquidity := ScoreLiquidity(t)
		catalyst := ScoreCatalyst(t)
		fundamental := ScoreFundamental(t)
		risk := ComputeRisk(t, days)

		// Weighted composite before risk penalty
		compositeRaw := momentum*w.Momentum +
			volatility*w.Volatility +
			liquidity*w.Liquidity +
			catalyst*w.Catalyst +
			fundamental*w.Fundamental

		// Apply risk penalty as a downward multiplier
		penaltyMultiplier := 1 - (risk.Penalty / 100 * w.RiskPenalty / 0.20) // scaled by weight
		if penaltyMultiplier < 0.5 {
			penaltyMultiplier = 0.5
		}
		composite := compositeRaw * penaltyMultiplier

		// Confidence: based on data completeness
		confidence, gaps := computeConfidence(t)

		result := ScoreResult{
			Ticker:           t.Ticker,
			CompanyName:      t.CompanyName,
			Sector:           t.Sector,
			CompositeScore:   clamp(composite),
			MomentumScore:    momentum,
			VolatilityScore:  volatility,
			LiquidityScore:   liquidity,
			CatalystScore:    catalyst,
			FundamentalScore: fundamental,
			RiskPenalty:      risk.Penalty,
			ConfidenceScore:  confidence,
			DataGaps:         gaps,
			RiskRating:       risk.Rating,
			Flags:            risk.Flags,
		}
		raw = append(raw, result)
	}

	// Sort by composite score descending
	sort.Slice(raw, func(i, j int) bool {
		return raw[i].CompositeScore > raw[j].CompositeScore
	})

	// Enforce sector concentration limit
	sectorCount := make(map[string]int)
	filtered := make([]ScoreResult, 0, len(raw))
	for _, r := range raw {
		if maxPerSector > 0 && sectorCount[r.Sector] >= maxPerSector {
			continue // skip — sector is full
		}
		sectorCount[r.Sector]++
		filtered = append(filtered, r)
	}

	return filtered
}

// computeConfidence returns a confidence score (0–100) and list of missing data fields.
func computeConfidence(t *TickerInput) (float64, []string) {
	score := 100.0
	var gaps []string

	// Price data quality
	if len(t.PriceBars) < 200 {
		score -= 10
		gaps = append(gaps, "insufficient_price_history")
	}
	if t.LatestBarAgeDays > 2 {
		score -= 10
		gaps = append(gaps, "stale_price_data")
	}
	if len(t.PriceBars) < 50 {
		score -= 15
		gaps = append(gaps, "very_limited_price_data")
	}

	// Fundamentals
	if !t.HasFundamentals {
		score -= 20
		gaps = append(gaps, "no_fundamentals")
	}
	if t.HasFundamentals && t.FundamentalsAgeDays > 30 {
		score -= 8
		gaps = append(gaps, "stale_fundamentals")
	}

	// News
	if t.NewsCount7d == 0 {
		score -= 10
		gaps = append(gaps, "no_recent_news")
	}
	if t.LatestNewsAgeDays > 3 {
		score -= 8
		gaps = append(gaps, "stale_news")
	}

	// IV data
	if t.IVRank == 0 {
		score -= 5
		gaps = append(gaps, "no_iv_data")
	}

	// Volume data
	if t.AvgVol30d == 0 {
		score -= 10
		gaps = append(gaps, "no_volume_data")
	}

	// Earnings calendar
	if t.EarningsDaysAway < 0 {
		score -= 5
		gaps = append(gaps, "unknown_earnings_date")
	}

	if t.Sector == "" || t.Sector == "Unknown" {
		score -= 5
		gaps = append(gaps, "unknown_sector")
	}

	if score < 10 {
		score = 10
	}
	return score, gaps
}

// ExplainRankDifference generates a human-readable explanation of why ticker A
// is ranked above ticker B.
func ExplainRankDifference(a, b ScoreResult) string {
	diffs := []struct {
		name  string
		delta float64
	}{
		{"Momentum", a.MomentumScore - b.MomentumScore},
		{"Volatility", a.VolatilityScore - b.VolatilityScore},
		{"Liquidity", a.LiquidityScore - b.LiquidityScore},
		{"Catalyst", a.CatalystScore - b.CatalystScore},
		{"Fundamentals", a.FundamentalScore - b.FundamentalScore},
	}

	type diffItem struct {
		Name  string  `json:"name"`
		Delta float64 `json:"delta"`
	}

	aLeads := make([]diffItem, 0)
	bLeads := make([]diffItem, 0)

	for _, d := range diffs {
		if d.delta > 2 {
			aLeads = append(aLeads, diffItem{d.name, d.delta})
		} else if d.delta < -2 {
			bLeads = append(bLeads, diffItem{d.name, -d.delta})
		}
	}

	out, _ := json.Marshal(map[string]interface{}{
		"a_ticker":       a.Ticker,
		"b_ticker":       b.Ticker,
		"composite_diff": a.CompositeScore - b.CompositeScore,
		"a_leads_on":     aLeads,
		"b_leads_on":     bLeads,
	})
	return string(out)
}
