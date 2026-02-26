package scorer

import "math"

// ScoreMomentum computes the Momentum/Trend sub-score (0–100).
//
// Components:
//   MA alignment (0–30): price > 20 EMA > 50 EMA > 200 EMA
//   ROC (0–25):          10-day rate of change, normalized
//   RS (0–25):           relative strength percentile vs universe
//   MACD (0–20):         bullish cross + expanding histogram
//   RSI zone (0–10):     40–70 ideal, penalty for overbought
func ScoreMomentum(t *TickerInput) float64 {
	// MA alignment score: each correct order = 10 pts
	maScore := 0.0
	if t.Price > t.EMA20 {
		maScore += 10
	}
	if t.EMA20 > t.EMA50 {
		maScore += 10
	}
	if t.EMA50 > t.EMA200 {
		maScore += 10
	}

	// Rate of change (10d): normalize from [-20%, +20%] → [0, 25]
	rocScore := normalize(t.ROC10, -20, 20) * 0.25

	// Relative strength percentile → [0, 25]
	rsScore := clamp(t.RelStrength52w) * 0.25

	// MACD score: bullish cross = 10, expanding histogram = 10
	macdScore := 0.0
	if t.MACDLine > t.MACDSignal {
		macdScore += 10
	}
	if t.MACDHistogram > 0 {
		macdScore += 10
	}

	// RSI zone: ideal 40–70 = full points, penalty for overbought
	rsiScore := 0.0
	switch {
	case t.RSI14 >= 40 && t.RSI14 <= 70:
		rsiScore = 10
	case t.RSI14 > 70 && t.RSI14 <= 80:
		rsiScore = 5
	case t.RSI14 > 80:
		rsiScore = 0 // overbought
	case t.RSI14 >= 30 && t.RSI14 < 40:
		rsiScore = 7 // approaching oversold — potential reversal
	default:
		rsiScore = 3 // deeply oversold
	}

	total := maScore + rocScore + rsScore + macdScore + rsiScore
	return clamp(total)
}

// ScoreVolatility computes the Volatility/Opportunity sub-score (0–100).
//
// Rewards tickers in a "goldilocks" volatility zone: enough to trade,
// not so much that it's just noise.
func ScoreVolatility(t *TickerInput) float64 {
	// ATR% target zone: 1.5%–5% for daily, but we score generically
	// and the horizon weights handle the rest.
	atrScore := 0.0
	switch {
	case t.ATR14Pct >= 1.5 && t.ATR14Pct <= 5:
		atrScore = 40
	case t.ATR14Pct > 5 && t.ATR14Pct <= 8:
		atrScore = 25
	case t.ATR14Pct > 8:
		atrScore = 10 // too volatile
	case t.ATR14Pct >= 0.5 && t.ATR14Pct < 1.5:
		atrScore = 20 // boring
	default:
		atrScore = 5
	}

	// IV Rank: lower is better (cheaper opportunity)
	ivScore := clamp(100-t.IVRank) * 0.30

	// BB squeeze: low width = coiled energy
	bbScore := clamp(100-t.BBWidth) * 0.30

	total := atrScore + ivScore + bbScore
	return clamp(total)
}

// ScoreLiquidity computes the Liquidity sub-score (0–100).
//
// Ensures the ticker is practically tradeable with tight spreads.
func ScoreLiquidity(t *TickerInput) float64 {
	// Average volume: log scale, floor at 500k
	volScore := 0.0
	if t.AvgVol30d >= 500_000 {
		volScore = normalize(math.Log10(t.AvgVol30d), math.Log10(500_000), math.Log10(50_000_000)) * 40
	}

	// Spread: <0.1% = perfect, >0.5% = 0
	spreadScore := 0.0
	if t.SpreadEstPct <= 0.1 {
		spreadScore = 35
	} else if t.SpreadEstPct <= 0.3 {
		spreadScore = 20
	} else if t.SpreadEstPct <= 0.5 {
		spreadScore = 10
	}

	// Float: >50M shares preferred
	floatScore := normalize(t.Float, 5, 100) * 0.15

	// Price: prefer $10–$500
	priceScore := 0.0
	if t.Price >= 10 && t.Price <= 500 {
		priceScore = 10
	} else if t.Price > 500 {
		priceScore = 7
	} else if t.Price >= 5 {
		priceScore = 5
	}

	total := volScore + spreadScore + floatScore + priceScore
	return clamp(total)
}
