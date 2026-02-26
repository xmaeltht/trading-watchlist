package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Horizon represents a trading time horizon.
type Horizon string

const (
	HorizonDaily   Horizon = "daily"
	HorizonWeekly  Horizon = "weekly"
	HorizonMonthly Horizon = "monthly"
)

// TickerScore holds the full scored record for a ticker on a given horizon and run.
type TickerScore struct {
	ID             int64   `db:"id" json:"id"`
	RunID          string  `db:"run_id" json:"run_id"`
	Horizon        Horizon `db:"horizon" json:"horizon"`
	Ticker         string  `db:"ticker" json:"ticker"`
	CompanyName    string  `db:"company_name" json:"company_name"`
	Sector         string  `db:"sector" json:"sector"`
	Rank           int     `db:"rank" json:"rank"`
	CompositeScore float64 `db:"composite_score" json:"composite_score"`

	// Sub-scores (0–100)
	MomentumScore    float64 `db:"momentum_score" json:"momentum_score"`
	VolatilityScore  float64 `db:"volatility_score" json:"volatility_score"`
	LiquidityScore   float64 `db:"liquidity_score" json:"liquidity_score"`
	CatalystScore    float64 `db:"catalyst_score" json:"catalyst_score"`
	FundamentalScore float64 `db:"fundamental_score" json:"fundamental_score"`
	RiskPenalty      float64 `db:"risk_penalty" json:"risk_penalty"`

	// Confidence
	ConfidenceScore float64 `db:"confidence_score" json:"confidence_score"`
	DataGaps        string  `db:"data_gaps" json:"data_gaps"` // JSON array of missing fields

	// Generated content
	Thesis           string `db:"thesis" json:"thesis"`                   // LLM-generated markdown bullets
	TradePlanText    string `db:"trade_plan_text" json:"trade_plan_text"` // LLM-generated template
	InvalidationText string `db:"invalidation_text" json:"invalidation_text"`
	RiskRating       string `db:"risk_rating" json:"risk_rating"` // LOW | MEDIUM | HIGH
	Flags            string `db:"flags" json:"flags"`             // JSON array of flag strings

	// Raw signals (stored as JSON for flexibility)
	TechnicalSnapshot   string `db:"technical_snapshot" json:"technical_snapshot"`     // JSON
	FundamentalSnapshot string `db:"fundamental_snapshot" json:"fundamental_snapshot"` // JSON
	NewsSummary         string `db:"news_summary" json:"news_summary"`                 // JSON

	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// PriceBar is a daily OHLCV record.
type PriceBar struct {
	Ticker    string    `db:"ticker"`
	Date      time.Time `db:"date"`
	Open      float64   `db:"open"`
	High      float64   `db:"high"`
	Low       float64   `db:"low"`
	Close     float64   `db:"close"`
	Volume    int64     `db:"volume"`
	VWAP      float64   `db:"vwap"`
	CreatedAt time.Time `db:"created_at"`
}

// NewsItem is a single news article.
type NewsItem struct {
	ID          int64     `db:"id"`
	Ticker      string    `db:"ticker"`
	Headline    string    `db:"headline"`
	Source      string    `db:"source"`
	URL         string    `db:"url"`
	PublishedAt time.Time `db:"published_at"`
	Sentiment   float64   `db:"sentiment"` // -1 to +1
	CreatedAt   time.Time `db:"created_at"`
}

// Fundamentals holds the latest fundamental snapshot for a ticker.
type Fundamentals struct {
	Ticker           string    `db:"ticker"`
	RevenueGrowthYoY float64   `db:"revenue_growth_yoy"`
	EPSGrowthYoY     float64   `db:"eps_growth_yoy"`
	GrossMargin      float64   `db:"gross_margin"`
	OperatingMargin  float64   `db:"operating_margin"`
	PEForward        float64   `db:"pe_forward"`
	PEGRatio         float64   `db:"peg_ratio"`
	EVToEBITDA       float64   `db:"ev_to_ebitda"`
	DebtToEquity     float64   `db:"debt_to_equity"`
	FCFYield         float64   `db:"fcf_yield"`
	EPSRevisions30d  int       `db:"eps_revisions_30d"`
	UpdatedAt        time.Time `db:"updated_at"`
}

type CompanyProfile struct {
	Ticker      string    `db:"ticker"`
	CompanyName string    `db:"company_name"`
	Sector      string    `db:"sector"`
	Industry    string    `db:"industry"`
	Source      string    `db:"source"`
	UpdatedAt   time.Time `db:"updated_at"`
}

// Store wraps the database connection pool.
type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// Migrate runs all schema migrations.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.Exec(ctx, schema)
	return err
}

// SaveScores upserts a batch of scored tickers for a given run.
func (s *Store) SaveScores(ctx context.Context, scores []TickerScore) error {
	for _, sc := range scores {
		_, err := s.db.Exec(ctx, `
			INSERT INTO ticker_scores (
				run_id, horizon, ticker, company_name, sector, rank,
				composite_score, momentum_score, volatility_score, liquidity_score,
				catalyst_score, fundamental_score, risk_penalty,
				confidence_score, data_gaps, thesis, trade_plan_text,
				invalidation_text, risk_rating, flags,
				technical_snapshot, fundamental_snapshot, news_summary
			) VALUES (
				$1,$2,$3,$4,$5,$6,
				$7,$8,$9,$10,
				$11,$12,$13,
				$14,$15,$16,$17,
				$18,$19,$20,
				$21,$22,$23
			)
			ON CONFLICT (run_id, horizon, ticker) DO UPDATE SET
				rank = EXCLUDED.rank,
				composite_score = EXCLUDED.composite_score,
				thesis = EXCLUDED.thesis,
				trade_plan_text = EXCLUDED.trade_plan_text,
				flags = EXCLUDED.flags`,
			sc.RunID, sc.Horizon, sc.Ticker, sc.CompanyName, sc.Sector, sc.Rank,
			sc.CompositeScore, sc.MomentumScore, sc.VolatilityScore, sc.LiquidityScore,
			sc.CatalystScore, sc.FundamentalScore, sc.RiskPenalty,
			sc.ConfidenceScore, sc.DataGaps, sc.Thesis, sc.TradePlanText,
			sc.InvalidationText, sc.RiskRating, sc.Flags,
			sc.TechnicalSnapshot, sc.FundamentalSnapshot, sc.NewsSummary,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetWatchlist returns the latest top-N ranked tickers for a horizon.
func (s *Store) GetWatchlist(ctx context.Context, horizon Horizon, limit int) ([]TickerScore, error) {
	rows, err := s.db.Query(ctx, `
		SELECT ts.*
		FROM ticker_scores ts
		INNER JOIN (
			SELECT MAX(run_id) AS latest_run FROM ticker_scores WHERE horizon = $1
		) latest ON ts.run_id = latest.latest_run
		WHERE ts.horizon = $1
		ORDER BY ts.rank ASC
		LIMIT $2`,
		horizon, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scores []TickerScore
	for rows.Next() {
		var sc TickerScore
		if err := rows.Scan(
			&sc.ID, &sc.RunID, &sc.Horizon, &sc.Ticker, &sc.CompanyName,
			&sc.Sector, &sc.Rank, &sc.CompositeScore,
			&sc.MomentumScore, &sc.VolatilityScore, &sc.LiquidityScore,
			&sc.CatalystScore, &sc.FundamentalScore, &sc.RiskPenalty,
			&sc.ConfidenceScore, &sc.DataGaps, &sc.Thesis, &sc.TradePlanText,
			&sc.InvalidationText, &sc.RiskRating, &sc.Flags,
			&sc.TechnicalSnapshot, &sc.FundamentalSnapshot, &sc.NewsSummary,
			&sc.CreatedAt,
		); err != nil {
			return nil, err
		}
		scores = append(scores, sc)
	}
	return scores, rows.Err()
}

// GetTicker returns the latest score record for a single ticker on a horizon.
func (s *Store) GetTicker(ctx context.Context, horizon Horizon, ticker string) (*TickerScore, error) {
	var sc TickerScore
	err := s.db.QueryRow(ctx, `
		SELECT ts.*
		FROM ticker_scores ts
		INNER JOIN (
			SELECT MAX(run_id) AS latest_run FROM ticker_scores WHERE horizon = $1
		) latest ON ts.run_id = latest.latest_run
		WHERE ts.horizon = $1 AND ts.ticker = $2`,
		horizon, ticker,
	).Scan(
		&sc.ID, &sc.RunID, &sc.Horizon, &sc.Ticker, &sc.CompanyName,
		&sc.Sector, &sc.Rank, &sc.CompositeScore,
		&sc.MomentumScore, &sc.VolatilityScore, &sc.LiquidityScore,
		&sc.CatalystScore, &sc.FundamentalScore, &sc.RiskPenalty,
		&sc.ConfidenceScore, &sc.DataGaps, &sc.Thesis, &sc.TradePlanText,
		&sc.InvalidationText, &sc.RiskRating, &sc.Flags,
		&sc.TechnicalSnapshot, &sc.FundamentalSnapshot, &sc.NewsSummary,
		&sc.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &sc, nil
}

// SavePriceBars bulk-inserts OHLCV bars, ignoring duplicates.
func (s *Store) SavePriceBars(ctx context.Context, bars []PriceBar) error {
	for _, b := range bars {
		_, err := s.db.Exec(ctx, `
			INSERT INTO price_bars (ticker, date, open, high, low, close, volume, vwap)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (ticker, date) DO UPDATE SET
				close = EXCLUDED.close, volume = EXCLUDED.volume, vwap = EXCLUDED.vwap`,
			b.Ticker, b.Date, b.Open, b.High, b.Low, b.Close, b.Volume, b.VWAP,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetPriceBars returns the last N daily bars for a ticker.
func (s *Store) GetPriceBars(ctx context.Context, ticker string, days int) ([]PriceBar, error) {
	rows, err := s.db.Query(ctx, `
		SELECT ticker, date, open, high, low, close, volume, vwap, created_at
		FROM price_bars
		WHERE ticker = $1
		ORDER BY date DESC
		LIMIT $2`,
		ticker, days,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bars []PriceBar
	for rows.Next() {
		var b PriceBar
		if err := rows.Scan(&b.Ticker, &b.Date, &b.Open, &b.High, &b.Low, &b.Close, &b.Volume, &b.VWAP, &b.CreatedAt); err != nil {
			return nil, err
		}
		bars = append(bars, b)
	}
	return bars, rows.Err()
}

// SaveFundamentals upserts fundamental data for a ticker.
func (s *Store) SaveFundamentals(ctx context.Context, f Fundamentals) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO fundamentals (
			ticker, revenue_growth_yoy, eps_growth_yoy, gross_margin,
			operating_margin, pe_forward, peg_ratio, ev_to_ebitda,
			debt_to_equity, fcf_yield, eps_revisions_30d, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW())
		ON CONFLICT (ticker) DO UPDATE SET
			revenue_growth_yoy = EXCLUDED.revenue_growth_yoy,
			eps_growth_yoy     = EXCLUDED.eps_growth_yoy,
			gross_margin       = EXCLUDED.gross_margin,
			operating_margin   = EXCLUDED.operating_margin,
			pe_forward         = EXCLUDED.pe_forward,
			peg_ratio          = EXCLUDED.peg_ratio,
			ev_to_ebitda       = EXCLUDED.ev_to_ebitda,
			debt_to_equity     = EXCLUDED.debt_to_equity,
			fcf_yield          = EXCLUDED.fcf_yield,
			eps_revisions_30d  = EXCLUDED.eps_revisions_30d,
			updated_at         = NOW()`,
		f.Ticker, f.RevenueGrowthYoY, f.EPSGrowthYoY, f.GrossMargin,
		f.OperatingMargin, f.PEForward, f.PEGRatio, f.EVToEBITDA,
		f.DebtToEquity, f.FCFYield, f.EPSRevisions30d,
	)
	return err
}

func (s *Store) SaveCompanyProfile(ctx context.Context, p CompanyProfile) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO company_profiles (ticker, company_name, sector, industry, source, updated_at)
		VALUES ($1,$2,$3,$4,$5,NOW())
		ON CONFLICT (ticker) DO UPDATE SET
			company_name = EXCLUDED.company_name,
			sector = EXCLUDED.sector,
			industry = EXCLUDED.industry,
			source = EXCLUDED.source,
			updated_at = NOW()`,
		p.Ticker, p.CompanyName, p.Sector, p.Industry, p.Source,
	)
	return err
}

func (s *Store) GetCompanyProfile(ctx context.Context, ticker string) (*CompanyProfile, error) {
	var p CompanyProfile
	err := s.db.QueryRow(ctx, `
		SELECT ticker, company_name, sector, industry, source, updated_at
		FROM company_profiles
		WHERE ticker = $1`, ticker,
	).Scan(&p.Ticker, &p.CompanyName, &p.Sector, &p.Industry, &p.Source, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// SaveNewsItem inserts a single news item, ignoring duplicates by URL.
func (s *Store) SaveNewsItem(ctx context.Context, n NewsItem) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO news_items (ticker, headline, source, url, published_at, sentiment)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT DO NOTHING`,
		n.Ticker, n.Headline, n.Source, n.URL, n.PublishedAt, n.Sentiment,
	)
	return err
}

// GetRecentNews returns the last N news items for a ticker.
func (s *Store) GetRecentNews(ctx context.Context, ticker string, limit int) ([]NewsItem, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, ticker, headline, source, url, published_at, sentiment, created_at
		FROM news_items
		WHERE ticker = $1
		ORDER BY published_at DESC
		LIMIT $2`,
		ticker, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []NewsItem
	for rows.Next() {
		var n NewsItem
		if err := rows.Scan(&n.ID, &n.Ticker, &n.Headline, &n.Source, &n.URL, &n.PublishedAt, &n.Sentiment, &n.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, n)
	}
	return items, rows.Err()
}

func (s *Store) GetFundamentals(ctx context.Context, ticker string) (*Fundamentals, error) {
	var f Fundamentals
	err := s.db.QueryRow(ctx, `
		SELECT ticker, revenue_growth_yoy, eps_growth_yoy, gross_margin,
		       operating_margin, pe_forward, peg_ratio, ev_to_ebitda,
		       debt_to_equity, fcf_yield, eps_revisions_30d, updated_at
		FROM fundamentals
		WHERE ticker = $1`, ticker,
	).Scan(
		&f.Ticker, &f.RevenueGrowthYoY, &f.EPSGrowthYoY, &f.GrossMargin,
		&f.OperatingMargin, &f.PEForward, &f.PEGRatio, &f.EVToEBITDA,
		&f.DebtToEquity, &f.FCFYield, &f.EPSRevisions30d, &f.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

type RunStatus struct {
	ID          string     `json:"id"`
	Horizon     string     `json:"horizon"`
	Status      string     `json:"status"`
	Stage       string     `json:"stage"`
	Processed   int        `json:"processed"`
	Total       int        `json:"total"`
	TickerCount int        `json:"ticker_count"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	LastUpdate  time.Time  `json:"last_update"`
	ErrorMsg    string     `json:"error_msg,omitempty"`
}

func (s *Store) CreateRun(ctx context.Context, id, horizon string, total int) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO score_runs (id, horizon, status, stage, processed, total, ticker_count, started_at, last_update)
		VALUES ($1,$2,'running','init',0,$3,0,NOW(),NOW())
		ON CONFLICT (id) DO NOTHING`,
		id, horizon, total,
	)
	return err
}

func (s *Store) UpdateRunProgress(ctx context.Context, id, stage string, processed int) error {
	_, err := s.db.Exec(ctx, `
		UPDATE score_runs
		SET stage = $2, processed = $3, last_update = NOW()
		WHERE id = $1`, id, stage, processed,
	)
	return err
}

func (s *Store) CompleteRun(ctx context.Context, id string, tickerCount int, errMsg string) error {
	status := "success"
	if errMsg != "" {
		status = "failed"
	}
	_, err := s.db.Exec(ctx, `
		UPDATE score_runs
		SET status = $2, ticker_count = $3, error_msg = $4, stage = 'done', finished_at = NOW(), last_update = NOW()
		WHERE id = $1`,
		id, status, tickerCount, errMsg,
	)
	return err
}

func (s *Store) GetRun(ctx context.Context, id string) (*RunStatus, error) {
	var r RunStatus
	err := s.db.QueryRow(ctx, `
		SELECT id, horizon, status, stage, processed, total, ticker_count, started_at, finished_at, last_update, COALESCE(error_msg,'')
		FROM score_runs WHERE id = $1`, id,
	).Scan(&r.ID, &r.Horizon, &r.Status, &r.Stage, &r.Processed, &r.Total, &r.TickerCount, &r.StartedAt, &r.FinishedAt, &r.LastUpdate, &r.ErrorMsg)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) LatestRunByHorizon(ctx context.Context, horizon Horizon) (*RunStatus, error) {
	var r RunStatus
	err := s.db.QueryRow(ctx, `
		SELECT id, horizon, status, stage, processed, total, ticker_count, started_at, finished_at, last_update, COALESCE(error_msg,'')
		FROM score_runs
		WHERE horizon = $1
		ORDER BY started_at DESC
		LIMIT 1`, horizon,
	).Scan(&r.ID, &r.Horizon, &r.Status, &r.Stage, &r.Processed, &r.Total, &r.TickerCount, &r.StartedAt, &r.FinishedAt, &r.LastUpdate, &r.ErrorMsg)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

var schema = `
CREATE TABLE IF NOT EXISTS price_bars (
	id         BIGSERIAL PRIMARY KEY,
	ticker     TEXT NOT NULL,
	date       DATE NOT NULL,
	open       DOUBLE PRECISION,
	high       DOUBLE PRECISION,
	low        DOUBLE PRECISION,
	close      DOUBLE PRECISION NOT NULL,
	volume     BIGINT,
	vwap       DOUBLE PRECISION,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	UNIQUE(ticker, date)
);

CREATE INDEX IF NOT EXISTS idx_price_bars_ticker_date ON price_bars(ticker, date DESC);

CREATE TABLE IF NOT EXISTS fundamentals (
	ticker              TEXT PRIMARY KEY,
	revenue_growth_yoy  DOUBLE PRECISION,
	eps_growth_yoy      DOUBLE PRECISION,
	gross_margin        DOUBLE PRECISION,
	operating_margin    DOUBLE PRECISION,
	pe_forward          DOUBLE PRECISION,
	peg_ratio           DOUBLE PRECISION,
	ev_to_ebitda        DOUBLE PRECISION,
	debt_to_equity      DOUBLE PRECISION,
	fcf_yield           DOUBLE PRECISION,
	eps_revisions_30d   INT DEFAULT 0,
	updated_at          TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS news_items (
	id           BIGSERIAL PRIMARY KEY,
	ticker       TEXT NOT NULL,
	headline     TEXT NOT NULL,
	source       TEXT,
	url          TEXT,
	published_at TIMESTAMPTZ,
	sentiment    DOUBLE PRECISION DEFAULT 0,
	created_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS company_profiles (
	ticker       TEXT PRIMARY KEY,
	company_name TEXT,
	sector       TEXT,
	industry     TEXT,
	source       TEXT DEFAULT 'alpha_vantage',
	updated_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_news_ticker_published ON news_items(ticker, published_at DESC);

CREATE TABLE IF NOT EXISTS ticker_scores (
	id                    BIGSERIAL PRIMARY KEY,
	run_id                TEXT NOT NULL,
	horizon               TEXT NOT NULL,
	ticker                TEXT NOT NULL,
	company_name          TEXT,
	sector                TEXT,
	rank                  INT,
	composite_score       DOUBLE PRECISION,
	momentum_score        DOUBLE PRECISION,
	volatility_score      DOUBLE PRECISION,
	liquidity_score       DOUBLE PRECISION,
	catalyst_score        DOUBLE PRECISION,
	fundamental_score     DOUBLE PRECISION,
	risk_penalty          DOUBLE PRECISION,
	confidence_score      DOUBLE PRECISION,
	data_gaps             TEXT DEFAULT '[]',
	thesis                TEXT DEFAULT '',
	trade_plan_text       TEXT DEFAULT '',
	invalidation_text     TEXT DEFAULT '',
	risk_rating           TEXT DEFAULT 'MEDIUM',
	flags                 TEXT DEFAULT '[]',
	technical_snapshot    TEXT DEFAULT '{}',
	fundamental_snapshot  TEXT DEFAULT '{}',
	news_summary          TEXT DEFAULT '{}',
	created_at            TIMESTAMPTZ DEFAULT NOW(),
	UNIQUE(run_id, horizon, ticker)
);

CREATE INDEX IF NOT EXISTS idx_scores_horizon_run ON ticker_scores(horizon, run_id DESC);

CREATE TABLE IF NOT EXISTS score_runs (
	id          TEXT PRIMARY KEY,
	horizon     TEXT NOT NULL,
	status      TEXT DEFAULT 'running',
	stage       TEXT DEFAULT 'init',
	processed   INT DEFAULT 0,
	total       INT DEFAULT 0,
	started_at  TIMESTAMPTZ DEFAULT NOW(),
	finished_at TIMESTAMPTZ,
	last_update TIMESTAMPTZ DEFAULT NOW(),
	ticker_count INT DEFAULT 0,
	error_msg   TEXT
);

-- Deduplicate before adding uniqueness on URL to avoid migration failures.
DELETE FROM news_items a
USING news_items b
WHERE a.id > b.id
  AND a.url IS NOT NULL
  AND a.url = b.url;

CREATE UNIQUE INDEX IF NOT EXISTS idx_news_items_url_unique ON news_items(url) WHERE url IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_score_runs_horizon_started ON score_runs(horizon, started_at DESC);

ALTER TABLE score_runs ADD COLUMN IF NOT EXISTS stage TEXT DEFAULT 'init';
ALTER TABLE score_runs ADD COLUMN IF NOT EXISTS processed INT DEFAULT 0;
ALTER TABLE score_runs ADD COLUMN IF NOT EXISTS total INT DEFAULT 0;
ALTER TABLE score_runs ADD COLUMN IF NOT EXISTS last_update TIMESTAMPTZ DEFAULT NOW();
`
