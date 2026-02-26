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

// NewsIngestor fetches market news from Finnhub.
type NewsIngestor struct {
	apiKey string
	client *http.Client
	store  *store.Store
}

func NewNewsIngestor(apiKey string, s *store.Store) *NewsIngestor {
	return &NewsIngestor{
		apiKey: apiKey,
		client: &http.Client{Timeout: 20 * time.Second},
		store:  s,
	}
}

// finnhubNewsItem represents a single Finnhub news article.
type finnhubNewsItem struct {
	Category string `json:"category"`
	Datetime int64  `json:"datetime"`
	Headline string `json:"headline"`
	Source   string `json:"source"`
	URL      string `json:"url"`
	Summary  string `json:"summary"`
	Related  string `json:"related"` // comma-separated tickers
}

// IngestCompanyNews fetches news for a specific ticker from Finnhub.
func (n *NewsIngestor) IngestCompanyNews(ctx context.Context, ticker string, daysBack int) error {
	if n.apiKey == "" {
		slog.Warn("finnhub API key not set, skipping news ingestion", "ticker", ticker)
		return nil
	}

	to := time.Now()
	from := to.AddDate(0, 0, -daysBack)

	url := fmt.Sprintf(
		"https://finnhub.io/api/v1/company-news?symbol=%s&from=%s&to=%s&token=%s",
		ticker,
		from.Format("2006-01-02"),
		to.Format("2006-01-02"),
		n.apiKey,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("finnhub request failed for %s: %w", ticker, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		slog.Warn("finnhub rate limited", "ticker", ticker)
		return fmt.Errorf("rate limited by finnhub")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("finnhub error %d for %s: %s", resp.StatusCode, ticker, string(body))
	}

	var items []finnhubNewsItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return fmt.Errorf("finnhub decode error for %s: %w", ticker, err)
	}

	// Convert and save (sentiment scored later by LLM or simple heuristic)
	for _, item := range items {
		newsItem := store.NewsItem{
			Ticker:      ticker,
			Headline:    item.Headline,
			Source:      item.Source,
			URL:         item.URL,
			PublishedAt: time.Unix(item.Datetime, 0),
			Sentiment:   0, // scored later
		}
		// Simple inline store — in production, batch this
		_ = n.store.SaveNewsItem(ctx, newsItem)
	}

	slog.Info("ingested news", "ticker", ticker, "count", len(items))
	return nil
}

// IngestUniverse ingests news for all tickers with rate limiting.
func (n *NewsIngestor) IngestUniverse(ctx context.Context, tickers []string, daysBack int) error {
	for i, ticker := range tickers {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := n.IngestCompanyNews(ctx, ticker, daysBack); err != nil {
			slog.Warn("news ingestion failed", "ticker", ticker, "error", err)
		}

		// Finnhub: 60 req/min → ~1s between calls
		if n.apiKey != "" && i < len(tickers)-1 {
			time.Sleep(1100 * time.Millisecond)
		}
	}
	return nil
}
