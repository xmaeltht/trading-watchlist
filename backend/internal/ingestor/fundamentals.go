package ingestor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/xmaeltht/trading-watchlist/internal/store"
)

// FundamentalsIngestor fetches fundamental data from Alpha Vantage.
type FundamentalsIngestor struct {
	apiKey string
	client *http.Client
	store  *store.Store
}

func NewFundamentalsIngestor(apiKey string, s *store.Store) *FundamentalsIngestor {
	return &FundamentalsIngestor{
		apiKey: apiKey,
		client: &http.Client{Timeout: 30 * time.Second},
		store:  s,
	}
}

// avOverviewResponse is the Alpha Vantage OVERVIEW endpoint shape.
type avOverviewResponse struct {
	Symbol                     string `json:"Symbol"`
	Name                       string `json:"Name"`
	Sector                     string `json:"Sector"`
	Industry                   string `json:"Industry"`
	ForwardPE                  string `json:"ForwardPE"`
	PEGRatio                   string `json:"PEGRatio"`
	EVToEBITDA                 string `json:"EVToEBITDA"`
	GrossProfitTTM             string `json:"GrossProfitTTM"`
	OperatingMarginTTM         string `json:"OperatingMarginTTM"`
	RevenueGrowthYOY           string `json:"RevenueGrowthYOY"`
	EPSGrowthYOY               string `json:"EPSGrowthYOY"`
	DebtToEquityRatio          string `json:"DebtToEquityRatio"`
	BookValue                  string `json:"BookValue"`
	SharesOutstanding          string `json:"SharesOutstanding"`
	EPS                        string `json:"EPS"`
	AnalystTargetPrice         string `json:"AnalystTargetPrice"`
	QuarterlyEarningsGrowthYOY string `json:"QuarterlyEarningsGrowthYOY"`
	QuarterlyRevenueGrowthYOY  string `json:"QuarterlyRevenueGrowthYOY"`
}

// IngestTicker fetches fundamental overview for a single ticker.
func (f *FundamentalsIngestor) IngestTicker(ctx context.Context, ticker string) error {
	if f.apiKey == "" {
		slog.Warn("alpha vantage API key not set, skipping fundamentals", "ticker", ticker)
		return nil
	}

	url := fmt.Sprintf(
		"https://www.alphavantage.co/query?function=OVERVIEW&symbol=%s&apikey=%s",
		ticker, f.apiKey,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("alpha vantage request failed for %s: %w", ticker, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("rate limited by alpha vantage")
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("alpha vantage error %d for %s: %s", resp.StatusCode, ticker, string(b))
	}

	var data avOverviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("alpha vantage decode error for %s: %w", ticker, err)
	}

	// Alpha Vantage returns an empty symbol if ticker not found or limit hit
	if data.Symbol == "" {
		slog.Warn("alpha vantage: empty response (likely rate limited or unknown ticker)", "ticker", ticker)
		return nil
	}

	fund := store.Fundamentals{
		Ticker:           ticker,
		RevenueGrowthYoY: parseFloat(data.QuarterlyRevenueGrowthYOY) * 100,
		EPSGrowthYoY:     parseFloat(data.QuarterlyEarningsGrowthYOY) * 100,
		GrossMargin:      parseFloat(data.OperatingMarginTTM) * 100,
		OperatingMargin:  parseFloat(data.OperatingMarginTTM) * 100,
		PEForward:        parseFloat(data.ForwardPE),
		PEGRatio:         parseFloat(data.PEGRatio),
		EVToEBITDA:       parseFloat(data.EVToEBITDA),
		DebtToEquity:     parseFloat(data.DebtToEquityRatio),
	}

	if err := f.store.SaveFundamentals(ctx, fund); err != nil {
		return fmt.Errorf("failed to save fundamentals for %s: %w", ticker, err)
	}
	_ = f.store.SaveCompanyProfile(ctx, store.CompanyProfile{
		Ticker:      ticker,
		CompanyName: data.Name,
		Sector:      data.Sector,
		Industry:    data.Industry,
		Source:      "alpha_vantage",
	})

	slog.Info("ingested fundamentals", "ticker", ticker, "pe_fwd", fund.PEForward)
	return nil
}

// IngestUniverse fetches fundamentals for all tickers with rate limiting.
// Alpha Vantage free tier: 5 req/min, 500 req/day.
func (f *FundamentalsIngestor) IngestUniverse(ctx context.Context, tickers []string) error {
	for i, ticker := range tickers {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := f.IngestTicker(ctx, ticker); err != nil {
			slog.Warn("fundamentals ingestion failed", "ticker", ticker, "error", err)
		}

		// 5 req/min → 12s between calls
		if f.apiKey != "" && i < len(tickers)-1 {
			time.Sleep(13 * time.Second)
		}
	}
	return nil
}

func parseFloat(s string) float64 {
	if s == "" || s == "None" || s == "-" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}
