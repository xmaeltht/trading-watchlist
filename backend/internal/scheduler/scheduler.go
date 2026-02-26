package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/xmaeltht/trading-watchlist/internal/config"
	"github.com/xmaeltht/trading-watchlist/internal/explainer"
	"github.com/xmaeltht/trading-watchlist/internal/ingestor"
	"github.com/xmaeltht/trading-watchlist/internal/scorer"
	"github.com/xmaeltht/trading-watchlist/internal/store"
)

// DefaultUniverse is the screening universe for MVP.
// Phase 2: dynamically load S&P500 + NDX + RUT1000 constituents from an API.
var DefaultUniverse = []string{
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
	fundIng   *ingestor.FundamentalsIngestor
	explainer *explainer.Explainer
}

func New(
	cfg *config.Config,
	s *store.Store,
	priceIng *ingestor.PriceIngestor,
	newsIng *ingestor.NewsIngestor,
	fundIng *ingestor.FundamentalsIngestor,
	exp *explainer.Explainer,
) *Scheduler {
	return &Scheduler{
		cron:      cron.New(cron.WithLocation(mustLoadLocation("America/New_York"))),
		cfg:       cfg,
		store:     s,
		priceIng:  priceIng,
		newsIng:   newsIng,
		fundIng:   fundIng,
		explainer: exp,
	}
}

func (s *Scheduler) Start() {
	// Daily pre-market: Mon–Fri 6:00 AM ET
	s.cron.AddFunc("0 6 * * 1-5", func() {
		slog.Info("starting daily scoring run")
		s.runScoring(store.HorizonDaily)
	})

	// Weekly: Sunday 6:00 PM ET
	s.cron.AddFunc("0 18 * * 0", func() {
		slog.Info("starting weekly scoring run")
		s.runScoring(store.HorizonWeekly)
	})

	// Monthly: 28th of month at 6:00 PM ET
	s.cron.AddFunc("0 18 28 * *", func() {
		slog.Info("starting monthly scoring run")
		s.runScoring(store.HorizonMonthly)
	})

	// News ingestion: every 4h on weekdays
	s.cron.AddFunc("0 */4 * * 1-5", func() {
		s.ingestNews()
	})

	// Fundamentals ingestion: daily at 5:00 AM ET (before scoring)
	s.cron.AddFunc("0 5 * * 1-5", func() {
		s.ingestFundamentals()
	})

	s.cron.Start()
	slog.Info("scheduler started")
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// RunNow triggers an immediate scoring run for a given horizon (manual/testing).
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

	slog.Info("scoring run started", "run_id", runID, "horizon", horizon, "universe", len(universe))

	// Step 1: Ingest latest prices
	if err := s.priceIng.IngestUniverse(ctx, universe, 250); err != nil {
		slog.Warn("partial price ingestion failure", "error", err)
	}

	// Step 2: Build scorer inputs
	inputMap := make(map[string]scorer.TickerInput, len(universe))
	inputs := make([]scorer.TickerInput, 0, len(universe))

	for _, ticker := range universe {
		bars, err := s.store.GetPriceBars(ctx, ticker, 250)
		if err != nil || len(bars) < 20 {
			slog.Warn("insufficient data, skipping ticker", "ticker", ticker)
			continue
		}

		input := scorer.TickerInput{
			Ticker:          ticker,
			PriceBars:       bars,
			Price:           bars[0].Close,
			EarningsDaysAway: -1, // unknown until calendar ingestor added
		}
		computeTechFeatures(&input)

		// Attach fundamentals if available
		// TODO: load from store.GetFundamentals once implemented
		inputMap[ticker] = input
		inputs = append(inputs, input)
	}

	// Step 3: Score and rank
	results := scorer.ScoreAll(inputs, horizon, s.cfg.MaxPerSector)
	if len(results) > s.cfg.ListSize {
		results = results[:s.cfg.ListSize]
	}

	// Step 4: Generate LLM explanations for top results
	explanations := s.explainer.ExplainBatch(ctx, results, inputMap, horizon)

	// Step 5: Save to store
	scores := make([]store.TickerScore, 0, len(results))
	for i, r := range results {
		exp := explanations[r.Ticker]
		dataGapsJSON, _ := marshalJSON(r.DataGaps)
		flagsJSON, _ := marshalJSON(r.Flags)

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
			DataGaps:         dataGapsJSON,
			Thesis:           exp.Thesis,
			TradePlanText:    exp.TradePlanText,
			InvalidationText: exp.InvalidationText,
			RiskRating:       r.RiskRating,
			Flags:            flagsJSON,
		})
	}

	if err := s.store.SaveScores(ctx, scores); err != nil {
		return fmt.Errorf("failed to save scores: %w", err)
	}

	slog.Info("scoring run complete", "run_id", runID, "tickers_saved", len(scores))
	return nil
}

func (s *Scheduler) ingestNews() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if err := s.newsIng.IngestUniverse(ctx, s.getUniverse(), 7); err != nil {
		slog.Error("news ingestion failed", "error", err)
	}
}

func (s *Scheduler) ingestFundamentals() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	if err := s.fundIng.IngestUniverse(ctx, s.getUniverse()); err != nil {
		slog.Error("fundamentals ingestion failed", "error", err)
	}
}

func (s *Scheduler) getUniverse() []string {
	extra := s.cfg.ExtraUniverseTickers()
	if len(extra) == 0 {
		return DefaultUniverse
	}
	return append(DefaultUniverse, extra...)
}

// computeTechFeatures populates TickerInput technical indicators from price bars.
// Production: replace with a proper TA library (markcheno/go-talib).
func computeTechFeatures(t *scorer.TickerInput) {
	bars := t.PriceBars
	n := len(bars)
	if n < 20 {
		return
	}

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

	if n >= 15 {
		gains, losses := 0.0, 0.0
		for i := 0; i < 14; i++ {
			diff := closes[i] - closes[i+1]
			if diff > 0 {
				gains += diff
			} else {
				losses -= diff
			}
		}
		if losses == 0 {
			t.RSI14 = 100
		} else {
			rs := (gains / 14) / (losses / 14)
			t.RSI14 = 100 - (100 / (1 + rs))
		}
	}

	if n >= 11 {
		t.ROC10 = ((closes[0] - closes[10]) / closes[10]) * 100
	}

	if n >= 30 {
		vol := 0.0
		for i := 0; i < 30; i++ {
			vol += float64(bars[i].Volume)
		}
		t.AvgVol30d = vol / 30
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

func marshalJSON(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]", err
	}
	return string(b), nil
}
