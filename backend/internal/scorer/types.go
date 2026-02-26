package scorer

import "github.com/xmaeltht/trading-watchlist/internal/store"

// Horizon weights per scoring component.
type HorizonWeights struct {
	Momentum    float64
	Volatility  float64
	Liquidity   float64
	Catalyst    float64
	Fundamental float64
	RiskPenalty float64 // penalty multiplier weight
}

var Weights = map[store.Horizon]HorizonWeights{
	store.HorizonDaily: {
		Momentum:    0.30,
		Volatility:  0.20,
		Liquidity:   0.15,
		Catalyst:    0.20,
		Fundamental: 0.05,
		RiskPenalty: 0.10,
	},
	store.HorizonWeekly: {
		Momentum:    0.25,
		Volatility:  0.15,
		Liquidity:   0.10,
		Catalyst:    0.20,
		Fundamental: 0.15,
		RiskPenalty: 0.15,
	},
	store.HorizonMonthly: {
		Momentum:    0.15,
		Volatility:  0.10,
		Liquidity:   0.05,
		Catalyst:    0.15,
		Fundamental: 0.35,
		RiskPenalty: 0.20,
	},
}

// TickerInput is the raw feature data fed into the scorer.
type TickerInput struct {
	Ticker      string
	CompanyName string
	Sector      string
	Price       float64
	PriceBars   []store.PriceBar // most recent first; need ≥200 bars for full scoring

	// Computed technicals (populated by TechFeatures)
	EMA20, EMA50, EMA200 float64
	RSI14                float64
	MACDLine, MACDSignal float64
	MACDHistogram        float64
	ATR14                float64 // absolute
	ATR14Pct             float64 // ATR14/price
	BBWidth              float64 // Bollinger Band width percentile (0–100)
	ROC10                float64 // 10-day rate of change (%)
	RelStrength52w       float64 // percentile vs universe (0–100)

	// Volume
	AvgVol30d    float64
	CurrentVol   float64
	Float        float64 // shares float (millions)
	SpreadEstPct float64 // estimated bid-ask spread %

	// Fundamentals
	Fundamentals    store.Fundamentals
	HasFundamentals bool

	// News & catalyst
	NewsSentiment7d  float64 // avg sentiment last 7 days (-1 to +1)
	NewsCount7d      int
	EarningsDaysAway int  // -1 = unknown
	AnalystUpgrades  int  // upgrades in last 30d
	UnusualOptions   bool // unusual options activity flag

	// Risk signals
	IVRank          float64 // 0–100
	DrawdownFrom52w float64 // % below 52-week high (positive = below)
	SocialSpikeZ    float64 // z-score of social mentions vs 30d baseline

	// Data freshness / regime context
	LatestBarAgeDays    int
	LatestNewsAgeDays   int
	FundamentalsAgeDays int
	MarketRegime        string // BULL | NEUTRAL | BEAR
}

// ScoreResult is the fully scored output for one ticker.
type ScoreResult struct {
	Ticker           string
	CompanyName      string
	Sector           string
	CompositeScore   float64
	MomentumScore    float64
	VolatilityScore  float64
	LiquidityScore   float64
	CatalystScore    float64
	FundamentalScore float64
	RiskPenalty      float64
	ConfidenceScore  float64
	DataGaps         []string
	RiskRating       string // LOW | MEDIUM | HIGH
	Flags            []string

	CurrentPrice       float64
	ProjectedTarget    float64
	ProjectedStop      float64
	ProjectedRR        float64
	ProjectedUpsidePct float64
}

// clamp ensures a value stays within [0, 100].
func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// normalize maps v from [min,max] to [0,100].
func normalize(v, min, max float64) float64 {
	if max == min {
		return 50
	}
	return clamp((v - min) / (max - min) * 100)
}
