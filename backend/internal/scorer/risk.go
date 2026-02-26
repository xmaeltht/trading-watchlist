package scorer

// RiskResult holds the risk penalty calculation output.
type RiskResult struct {
	Penalty float64  // 0–30 (subtracted from composite)
	Rating  string   // LOW | MEDIUM | HIGH
	Flags   []string // visible flags for the UI
}

// ComputeRisk evaluates risk signals and returns a penalty and flags.
//
// Penalties:
//
//	Earnings within horizon window:  -5 to -15
//	Deep drawdown (>20% from 52w):   -5
//	Low-float spike (no news):       -10
//	High IV rank (>85):              -8
//	Social sentiment spike (>3σ):    -5
//
// Penalty capped at 30.
func ComputeRisk(t *TickerInput, horizonDays int) RiskResult {
	var penalty float64
	var flags []string

	// Earnings risk: scaled by horizon proximity
	if t.EarningsDaysAway >= 0 && t.EarningsDaysAway <= horizonDays {
		switch {
		case t.EarningsDaysAway <= 2:
			penalty += 15
			flags = append(flags, "🟡 EARNINGS IN "+itoa(t.EarningsDaysAway)+"D")
		case t.EarningsDaysAway <= 7:
			penalty += 10
			flags = append(flags, "🟡 EARNINGS IN "+itoa(t.EarningsDaysAway)+"D")
		default:
			penalty += 5
			flags = append(flags, "🟡 EARNINGS IN "+itoa(t.EarningsDaysAway)+"D")
		}
	}

	// Drawdown from 52-week high
	if t.DrawdownFrom52w > 20 {
		penalty += 5
	}

	// High IV crush risk
	if t.IVRank > 85 {
		penalty += 8
		flags = append(flags, "⚡ HIGH IV RANK")
	} else if t.IVRank > 70 {
		penalty += 3
		flags = append(flags, "⚡ IV RANK "+itoa(int(t.IVRank)))
	}

	// Social media hype spike
	if t.SocialSpikeZ > 3 {
		penalty += 5
		flags = append(flags, "📢 SOCIAL SPIKE")
	}
	// Fallback when sentiment feed is unavailable: abnormal raw news volume
	if t.SocialSpikeZ == 0 && t.NewsCount7d >= 12 {
		penalty += 4
		flags = append(flags, "📰 NEWS SPIKE")
	}

	// Regime guardrail: penalize risk in bearish market context
	if t.MarketRegime == "BEAR" {
		penalty += 5
		flags = append(flags, "🌧️ BEAR REGIME")
	} else if t.MarketRegime == "NEUTRAL" {
		penalty += 2
	}

	// Low-float momentum trap: price up >30% in 5d with no news
	if len(t.PriceBars) >= 5 && t.Price > 0 {
		fiveDayReturn := (t.PriceBars[0].Close/t.PriceBars[4].Close - 1) * 100
		if fiveDayReturn > 30 && t.NewsCount7d == 0 {
			penalty += 10
			flags = append(flags, "🔴 MOMENTUM TRAP FLAG")
		}
	}

	// Cap penalty
	if penalty > 30 {
		penalty = 30
	}

	// Determine rating
	rating := "LOW"
	switch {
	case penalty >= 15:
		rating = "HIGH"
	case penalty >= 7:
		rating = "MEDIUM"
	}

	return RiskResult{
		Penalty: penalty,
		Rating:  rating,
		Flags:   flags,
	}
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
