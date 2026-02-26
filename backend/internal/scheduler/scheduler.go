package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/xmaeltht/trading-watchlist/internal/config"
	"github.com/xmaeltht/trading-watchlist/internal/ingestor"
	"github.com/xmaeltht/trading-watchlist/internal/scorer"
	"github.com/xmaeltht/trading-watchlist/internal/store"
)

// Universe is the default set of tickers to screen.
// In production, this would be dynamically loaded from SP500 + NDX + RUT1000 constituents.
var DefaultUniverse = []string{
	// Top 50 by market cap (placeholder — will be dynamically loaded in v2)
	"AAPL", "MSFT", "GOOG", "GOOGL", "AMZN", "NVDA", "META", "BRK.B", "TSLA", "UNH",
	"XOM", "JNJ", "JPM", "V", "PG", "MA", "HD", "AVGO", "CVX", "MRK",
	"ABBV", "LLY", "PEP", "KO", "COST", "ADBE", "WMT", "BAC", "CRM", "TMO",
	"MCD", "CSCO", "ACN", "ABT", "NFLX", "LIN", "DHR", "TXN", "PM", "NEE",
	"CMCSA", "ORCL", "AMD", "UNP", "INTC", "IBM", "QCOM", "LOW", "SBUX", "GS",
}

type Scheduler struct {
	cron      *cron.Cron
	cfg       *config.Config
	store     *store.Store
	priceIng  *ingestor.PriceIngestor
	newsIng   *ingestor.NewsIngestor
}

func New(cfg *config.Config, s *store.Store, priceIng *ingestor.PriceIngestor, newsIng *ingestor.NewsIngestor) *Scheduler {
	return &Scheduler{
		cron:     cron.New(cron.WithLocation(mustLoadLocation("America/New_York"))),
		cfg:      cfg,
		store:    s,
		priceIng: priceIng,
		newsIng:  newsIng,
	}
}

func (s *Scheduler) Start() {
	// Daily: 6:00 AM ET (pre-market)
	s.cron.AddFunc("0 6 * * 1-5", func() {
		slog.Info("starting daily scoring run")
		s.runScoring(store.HorizonDaily)
	})

	// Weekly: Sunday 6:00 PM ET
	s.cron.AddFunc("0 18 * * 0", func() {
		slog.Info("starting weekly scoring run")
		s.runScoring(store.HorizonWeekly)
	})

	// Monthly: Last day of month at 6:00 PM ET (simplified: 28th)
	s.cron.AddFunc("0 18 28 * *", func() {
		slog.Info("starting monthly scoring run")
		s.runScoring(store.HorizonMonthly)
	})

	// Data ingestion: every 4 hours on weekdays (news)
	s.cron.AddFunc("0 */4 * * 1-5", func() {
		slog.Info("starting news ingestion")
		s.ingestNews()
	})

	s.cron.Start()
	slog.Info("scheduler started")
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// RunNow triggers an immediate scoring run for all horizons (for testing/manual trigger).
func (s *Scheduler) RunNow(ctx context.Context, horizon store.Horizon) error {
	return s.runScoringCtx(ctx, horizon)
}

func (s *Scheduler) runScoring(horizon store.Horizon) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := s.runScoringCtx(ctx, horizon); err != nil {
		slog.Error("scoring run failed", "horizon", horizon, "error", err)
	}
}

func (s *Scheduler) runScoringCtx(ctx context.Context, horizon store.Horizon) error {
	runID := fmt.Sprintf("%s_%s", horizon, time.Now().Format("20060102_150405"))
	universe := s.getUniverse()

	slog.Info("scoring run started", "run_id", runID, "horizon", horizon, "universe_size", len(universe))

	// Step 1: Ingest latest prices
	daysNeeded := 250 // enough for 200-day EMA
	if err := s.priceIng.IngestUniverse(ctx, universe, daysNeeded); err != nil {
		slog.Warn("partial price ingestion failure", "error", err)
	}

	// Step 2: Build scorer inputs from stored data
	inputs := make([]scorer.TickerInput, 0, len(universe))
	for _, ticker := range universe {
		bars, err := s.store.GetPriceBars(ctx, ticker, daysNeeded)
		if err != nil || len(bars) < 20 {
			slog.Warn("insufficient data for ticker, skipping", "ticker", ticker)
			continue
		}

		input := scorer.TickerInput{
			Ticker:    ticker,
			PriceBars: bars,
			Price:     bars[0].Close,
		}

		// Compute technical features from price bars
		computeTechFeatures(&input)

		inputs = append(inputs, input)
	}

	// Step 3: Score all tickers
	results := scorer.ScoreAll(inputs, horizon, s.cfg.MaxPerSector)

	// Step 4: Convert to store records and save
	scores := make([]store.TickerScore, 0, len(results))
	for i, r := range results {
		if i >= s.cfg.ListSize {
			break
		}
		scores = append(scores, store.TickerScore{
			RunID:            runID,
			Horizon:          horizon,
			Ticker:           r.Ticker,
			CompanyName:      r.CompanyName,
			Sector:           r.Sector,
			Rank:             i + 1,
			CompositeScore:   r.CompositeScore,
			MomentumScore:    r.MomentumScore,
			VolatilityScore:  r.VolatilityScore,
			LiquidityScore:   r.LiquidityScore,
			CatalystScore:    r.CatalystScore,
			FundamentalScore: r.FundamentalScore,
			RiskPenalty:      r.RiskPenalty,
			ConfidenceScore:  r.ConfidenceScore,
			RiskRating:       r.RiskRating,
		})
	}

	if err := s.store.SaveScores(ctx, scores); err != nil {
		return fmt.Errorf("failed to save scores: %w", err)
	}

	slog.Info("scoring run completed", "run_id", runID, "scored", len(scores))
	return nil
}

func (s *Scheduler) ingestNews() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	universe := s.getUniverse()
	if err := s.newsIng.IngestUniverse(ctx, universe, 7); err != nil {
		slog.Error("news ingestion failed", "error", err)
	}
}

func (s *Scheduler) getUniverse() []string {
	extra := s.cfg.ExtraUniverseTickers()
	if len(extra) > 0 {
		return append(DefaultUniverse, extra...)
	}
	return DefaultUniverse
}

// computeTechFeatures populates technical indicator fields on a TickerInput
// from its PriceBars. This is a simplified version — production would use
// a proper TA library.
func computeTechFeatures(t *scorer.TickerInput) {
	bars := t.PriceBars
	n := len(bars)
	if n < 20 {
		return
	}

	// Simple EMA approximation using close prices (most recent first)
	// In production, use a proper TA library like markcheno/go-talib
	closes := make([]float64, n)
	for i, b := range bars {
		closes[i] = b.Close
	}

	t.EMA20 = simpleAvg(closes, 20)
	if n >= 50 {
		t.EMA50 = simpleAvg(closes, 50)
	}
	if n >= 200 {
		t.EMA200 = simpleAvg(closes, 200)
	}

	// ATR (simplified: average of high-low over 14 days)
	if n >= 14 {
		atrSum := 0.0
		for i := 0; i < 14; i++ {
			atrSum += bars[i].High - bars[i].Low
		}
		t.ATR14 = atrSum / 14
		if t.Price > 0 {
			t.ATR14Pct = (t.ATR14 / t.Price) * 100
		}
	}

	// RSI (simplified 14-period)
	if n >= 15 {
		gains, losses := 0.0, 0.0
		for i := 0; i < 14; i++ {
			diff := closes[i] - closes[i+1] // most recent first
			if diff > 0 {
				gains += diff
			} else {
				losses -= diff
			}
		}
		avgGain := gains / 14
		avgLoss := losses / 14
		if avgLoss == 0 {
			t.RSI14 = 100
		} else {
			rs := avgGain / avgLoss
			t.RSI14 = 100 - (100 / (1 + rs))
		}
	}

	// ROC (10-day)
	if n >= 11 {
		t.ROC10 = ((closes[0] - closes[10]) / closes[10]) * 100
	}

	// Volume
	if n >= 30 {
		volSum := 0.0
		for i := 0; i < 30; i++ {
			volSum += float64(bars[i].Volume)
		}
		t.AvgVol30d = volSum / 30
	}
	if n > 0 {
		t.CurrentVol = float64(bars[0].Volume)
	}
}

func simpleAvg(data []float64, period int) float64 {
	if len(data) < period {
		return 0
	}
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += data[i]
	}
	return sum / float64(period)
}

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(fmt.Sprintf("failed to load timezone %s: %v", name, err))
	}
	return loc
}
