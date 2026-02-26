package ingestor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/xmaeltht/trading-watchlist/internal/store"
)

// PriceIngestor fetches OHLCV data from Polygon.io and stores it.
type PriceIngestor struct {
	apiKey string
	client *http.Client
	store  *store.Store
}

func NewPriceIngestor(apiKey string, s *store.Store) *PriceIngestor {
	return &PriceIngestor{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
		store:  s,
	}
}

// polygonAggResponse is the Polygon.io /v2/aggs/ticker response shape.
type polygonAggResponse struct {
	Results []struct {
		T  int64   `json:"t"`  // timestamp ms
		O  float64 `json:"o"`  // open
		H  float64 `json:"h"`  // high
		L  float64 `json:"l"`  // low
		C  float64 `json:"c"`  // close
		V  float64 `json:"v"`  // volume
		VW float64 `json:"vw"` // VWAP
	} `json:"results"`
	Status string `json:"status"`
}

// IngestTicker fetches the last N days of daily bars for a ticker.
func (p *PriceIngestor) IngestTicker(ctx context.Context, ticker string, days int) error {
	if p.apiKey == "" {
		slog.Warn("polygon API key not set, skipping price ingestion", "ticker", ticker)
		return nil
	}

	to := time.Now()
	from := to.AddDate(0, 0, -days)

	url := fmt.Sprintf(
		"https://api.polygon.io/v2/aggs/ticker/%s/range/1/day/%s/%s?adjusted=true&sort=asc&limit=%d&apiKey=%s",
		ticker,
		from.Format("2006-01-02"),
		to.Format("2006-01-02"),
		days+10, // extra buffer for weekends
		p.apiKey,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("polygon request failed for %s: %w", ticker, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		slog.Warn("polygon rate limited", "ticker", ticker)
		return fmt.Errorf("rate limited by polygon")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("polygon error %d for %s: %s", resp.StatusCode, ticker, string(body))
	}

	var data polygonAggResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("polygon decode error for %s: %w", ticker, err)
	}

	bars := make([]store.PriceBar, 0, len(data.Results))
	for _, r := range data.Results {
		bars = append(bars, store.PriceBar{
			Ticker: ticker,
			Date:   time.UnixMilli(r.T).UTC().Truncate(24 * time.Hour),
			Open:   r.O,
			High:   r.H,
			Low:    r.L,
			Close:  r.C,
			Volume: int64(r.V),
			VWAP:   r.VW,
		})
	}

	if len(bars) > 0 {
		if err := p.store.SavePriceBars(ctx, bars); err != nil {
			return fmt.Errorf("failed to save price bars for %s: %w", ticker, err)
		}
		slog.Info("ingested price bars", "ticker", ticker, "count", len(bars))
	}

	return nil
}

// IngestUniverse ingests price data for a slice of tickers with rate limiting.
func (p *PriceIngestor) IngestUniverse(ctx context.Context, tickers []string, days int) error {
	for i, ticker := range tickers {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := p.IngestTicker(ctx, ticker, days); err != nil {
			slog.Warn("price ingestion failed", "ticker", ticker, "error", err)
			// Continue with next ticker — don't fail the whole batch
		}

		// Polygon free tier: 5 req/min → wait 12.5s between calls
		if p.apiKey != "" && i < len(tickers)-1 {
			time.Sleep(13 * time.Second)
		}
	}
	return nil
}
