package scorer

// ScoreCatalyst computes the Catalyst/News sub-score (0–100).
//
// Components:
//   Sentiment (0–40): average sentiment of recent headlines
//   Events (0–50):    earnings, analyst upgrades, unusual options
//   Credibility (0–10): source tier weighting (simplified)
func ScoreCatalyst(t *TickerInput) float64 {
	// Sentiment score: normalize from [-1, +1] to [0, 40]
	sentimentScore := normalize(t.NewsSentiment7d, -1, 1) * 0.40

	// Event score: accumulate based on catalysts present
	eventScore := 0.0
	if t.EarningsDaysAway >= 0 && t.EarningsDaysAway <= 14 {
		eventScore += 15 // upcoming earnings = catalyst (risky but newsworthy)
	}
	if t.AnalystUpgrades > 0 {
		eventScore += float64(min(t.AnalystUpgrades, 3)) * 10 // cap at 30
	}
	if t.UnusualOptions {
		eventScore += 15
	}
	if eventScore > 50 {
		eventScore = 50
	}

	// Credibility: simplified — more news from diverse sources = more credible
	credScore := 0.0
	if t.NewsCount7d >= 5 {
		credScore = 10
	} else if t.NewsCount7d >= 2 {
		credScore = 6
	} else if t.NewsCount7d >= 1 {
		credScore = 3
	}

	total := sentimentScore + eventScore + credScore
	return clamp(total)
}

// ScoreFundamental computes the Fundamental Quality sub-score (0–100).
//
// Heavier weight for monthly horizon (handled by horizon weights).
// Components:
//   Growth (0–30): revenue + EPS YoY
//   Margins (0–20): gross + operating margin vs typical
//   Valuation (0–25): PEG, EV/EBITDA
//   Balance sheet (0–15): debt/equity, FCF yield
//   Revisions (0–10): EPS estimate revisions
func ScoreFundamental(t *TickerInput) float64 {
	if !t.HasFundamentals {
		return 30 // baseline score when data unavailable
	}
	f := t.Fundamentals

	// Growth: normalize from [-20%, +100%] each
	revGrowth := normalize(f.RevenueGrowthYoY, -20, 100) * 0.15
	epsGrowth := normalize(f.EPSGrowthYoY, -20, 100) * 0.15

	// Margins: gross margin target 30%–80%, operating margin target 10%–50%
	marginScore := normalize(f.GrossMargin, 10, 80)*0.10 + normalize(f.OperatingMargin, 0, 50)*0.10

	// Valuation: lower PEG is better (0.5–2.0 ideal range)
	pegScore := 0.0
	if f.PEGRatio > 0 {
		pegScore = normalize(3-f.PEGRatio, 0, 3) * 0.15 // invert: lower PEG = better
	}
	evScore := normalize(20-f.EVToEBITDA, 0, 20) * 0.10 // invert: lower = better

	// Balance sheet
	debtScore := normalize(2-f.DebtToEquity, 0, 2) * 0.08 // lower debt/equity = better
	fcfScore := normalize(f.FCFYield, 0, 10) * 0.07

	// EPS revisions: positive = good
	revisionScore := normalize(float64(f.EPSRevisions30d), -5, 10) * 0.10

	total := (revGrowth + epsGrowth + marginScore + pegScore + evScore + debtScore + fcfScore + revisionScore) * 100
	return clamp(total)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
