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

func (s *Scheduler) runScoringCtx(ctx context.Context, horizon store.Horizon) (runErr error) {
	runID := fmt.Sprintf("%s_%s", horizon, time.Now().Format("20060102_150405"))
	universe := s.getUniverse()
	_ = s.store.CreateRun(ctx, runID, string(horizon), len(universe))
	defer func() {
		if runErr != nil {
			_ = s.store.CompleteRun(ctx, runID, 0, runErr.Error())
		}
	}()

	slog.Info("scoring run started", "run_id", runID, "horizon", horizon, "universe", len(universe))
	_ = s.store.UpdateRunProgress(ctx, runID, "ingest_prices", 0)

	// Step 1: Ingest latest prices (+ SPY benchmark for regime)
	priceUniverse := append([]string{}, universe...)
	priceUniverse = append(priceUniverse, "SPY")
	if err := s.priceIng.IngestUniverse(ctx, priceUniverse, 250); err != nil {
		slog.Warn("partial price ingestion failure", "error", err)
	}

	// Step 2: Build scorer inputs
	_ = s.store.UpdateRunProgress(ctx, runID, "build_inputs", 0)
	inputMap := make(map[string]scorer.TickerInput, len(universe))
	inputs := make([]scorer.TickerInput, 0, len(universe))
	regime := s.detectMarketRegime(ctx)

	for i, ticker := range universe {
		bars, err := s.store.GetPriceBars(ctx, ticker, 250)
		if err != nil || len(bars) < 20 {
			slog.Warn("insufficient data, skipping ticker", "ticker", ticker)
			_ = s.store.UpdateRunProgress(ctx, runID, "build_inputs", i+1)
			continue
		}

		company, sector := companySectorForTicker(ticker)
		if p, err := s.store.GetCompanyProfile(ctx, ticker); err == nil && p != nil {
			if p.CompanyName != "" {
				company = p.CompanyName
			}
			if p.Sector != "" {
				sector = p.Sector
			}
		}
		input := scorer.TickerInput{
			Ticker:           ticker,
			CompanyName:      company,
			Sector:           sector,
			PriceBars:        bars,
			Price:            bars[0].Close,
			EarningsDaysAway: -1,
			MarketRegime:     regime,
			LatestBarAgeDays: int(time.Since(bars[0].Date).Hours() / 24),
		}
		computeTechFeatures(&input)

		if f, err := s.store.GetFundamentals(ctx, ticker); err == nil && f != nil {
			input.Fundamentals = *f
			input.HasFundamentals = true
			input.FundamentalsAgeDays = int(time.Since(f.UpdatedAt).Hours() / 24)
		}
		if news, err := s.store.GetRecentNews(ctx, ticker, 25); err == nil {
			input.NewsCount7d = 0
			latest := 999
			for _, n := range news {
				age := int(time.Since(n.PublishedAt).Hours() / 24)
				if age < latest {
					latest = age
				}
				if age <= 7 {
					input.NewsCount7d++
				}
			}
			if latest != 999 {
				input.LatestNewsAgeDays = latest
			}
		}

		inputMap[ticker] = input
		inputs = append(inputs, input)
		_ = s.store.UpdateRunProgress(ctx, runID, "build_inputs", i+1)
	}

	// Step 3: Score and rank
	_ = s.store.UpdateRunProgress(ctx, runID, "score", len(universe))
	results := scorer.ScoreAll(inputs, horizon, s.cfg.MaxPerSector)
	if len(results) > s.cfg.ListSize {
		results = results[:s.cfg.ListSize]
	}

	// Step 4: Generate LLM explanations for top results
	_ = s.store.UpdateRunProgress(ctx, runID, "explain", len(universe))
	explanations := s.explainer.ExplainBatch(ctx, results, inputMap, horizon)

	// Step 5: Save to store
	_ = s.store.UpdateRunProgress(ctx, runID, "save", len(universe))
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
		runErr = fmt.Errorf("failed to save scores: %w", err)
		return runErr
	}
	_ = s.store.CompleteRun(ctx, runID, len(scores), "")

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

func (s *Scheduler) detectMarketRegime(ctx context.Context) string {
	bars, err := s.store.GetPriceBars(ctx, "SPY", 80)
	if err != nil || len(bars) < 50 {
		return "NEUTRAL"
	}
	closes := make([]float64, len(bars))
	for i, b := range bars {
		closes[i] = b.Close
	}
	ema20 := simpleAvg(closes, 20)
	ema50 := simpleAvg(closes, 50)
	if bars[0].Close < ema20 && ema20 < ema50 {
		return "BEAR"
	}
	if bars[0].Close > ema20 && ema20 > ema50 {
		return "BULL"
	}
	return "NEUTRAL"
}

var companySectorMap = map[string][2]string{
	"AAPL": {"Apple Inc.", "Technology"}, "MSFT": {"Microsoft Corporation", "Technology"}, "GOOG": {"Alphabet Inc.", "Communication Services"}, "GOOGL": {"Alphabet Inc.", "Communication Services"}, "AMZN": {"Amazon.com, Inc.", "Consumer Discretionary"},
	"NVDA": {"NVIDIA Corporation", "Technology"}, "META": {"Meta Platforms, Inc.", "Communication Services"}, "BRK.B": {"Berkshire Hathaway Inc.", "Financials"}, "TSLA": {"Tesla, Inc.", "Consumer Discretionary"}, "UNH": {"UnitedHealth Group Incorporated", "Health Care"},
	"XOM": {"Exxon Mobil Corporation", "Energy"}, "JNJ": {"Johnson & Johnson", "Health Care"}, "JPM": {"JPMorgan Chase & Co.", "Financials"}, "V": {"Visa Inc.", "Financials"}, "PG": {"The Procter & Gamble Company", "Consumer Staples"},
	"MA": {"Mastercard Incorporated", "Financials"}, "HD": {"The Home Depot, Inc.", "Consumer Discretionary"}, "AVGO": {"Broadcom Inc.", "Technology"}, "CVX": {"Chevron Corporation", "Energy"}, "MRK": {"Merck & Co., Inc.", "Health Care"},
	"ABBV": {"AbbVie Inc.", "Health Care"}, "LLY": {"Eli Lilly and Company", "Health Care"}, "PEP": {"PepsiCo, Inc.", "Consumer Staples"}, "KO": {"The Coca-Cola Company", "Consumer Staples"}, "COST": {"Costco Wholesale Corporation", "Consumer Staples"},
	"ADBE": {"Adobe Inc.", "Technology"}, "WMT": {"Walmart Inc.", "Consumer Staples"}, "BAC": {"Bank of America Corporation", "Financials"}, "CRM": {"Salesforce, Inc.", "Technology"}, "TMO": {"Thermo Fisher Scientific Inc.", "Health Care"},
	"MCD": {"McDonald's Corporation", "Consumer Discretionary"}, "CSCO": {"Cisco Systems, Inc.", "Technology"}, "ACN": {"Accenture plc", "Information Technology"}, "ABT": {"Abbott Laboratories", "Health Care"}, "NFLX": {"Netflix, Inc.", "Communication Services"},
	"LIN": {"Linde plc", "Materials"}, "DHR": {"Danaher Corporation", "Health Care"}, "TXN": {"Texas Instruments Incorporated", "Technology"}, "PM": {"Philip Morris International Inc.", "Consumer Staples"}, "NEE": {"NextEra Energy, Inc.", "Utilities"},
	"CMCSA": {"Comcast Corporation", "Communication Services"}, "ORCL": {"Oracle Corporation", "Technology"}, "AMD": {"Advanced Micro Devices, Inc.", "Technology"}, "UNP": {"Union Pacific Corporation", "Industrials"}, "INTC": {"Intel Corporation", "Technology"},
	"IBM": {"International Business Machines Corporation", "Technology"}, "QCOM": {"QUALCOMM Incorporated", "Technology"}, "LOW": {"Lowe's Companies, Inc.", "Consumer Discretionary"}, "SBUX": {"Starbucks Corporation", "Consumer Discretionary"}, "GS": {"The Goldman Sachs Group, Inc.", "Financials"},
}

func companySectorForTicker(ticker string) (company string, sector string) {
	if v, ok := companySectorMap[ticker]; ok {
		return v[0], v[1]
	}
	return ticker, "Unknown"
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
