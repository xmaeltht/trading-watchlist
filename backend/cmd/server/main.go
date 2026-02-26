package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xmaeltht/trading-watchlist/internal/api"
	"github.com/xmaeltht/trading-watchlist/internal/config"
	"github.com/xmaeltht/trading-watchlist/internal/explainer"
	"github.com/xmaeltht/trading-watchlist/internal/ingestor"
	"github.com/xmaeltht/trading-watchlist/internal/scheduler"
	"github.com/xmaeltht/trading-watchlist/internal/store"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Database
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	s := store.New(db)
	if err := s.Migrate(ctx); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("database ready")

	// Ingestors
	priceIng := ingestor.NewPriceIngestor(cfg.PolygonAPIKey, s)
	newsIng := ingestor.NewNewsIngestor(cfg.FinnhubAPIKey, s)
	fundIng := ingestor.NewFundamentalsIngestor(cfg.AlphaVantageAPIKey, s)

	// LLM explainer
	exp := explainer.New(cfg.LLMProvider, cfg.LLMModel, cfg.OllamaBaseURL, cfg.AnthropicAPIKey)

	// Scheduler
	sched := scheduler.New(cfg, s, priceIng, newsIng, fundIng, exp)
	sched.Start()
	defer sched.Stop()

	// API server
	srv := api.NewServer(cfg, s, sched)
	go func() {
		if err := srv.Listen(":" + cfg.Port); err != nil {
			slog.Error("server stopped", "error", err)
		}
	}()

	slog.Info("trading watchlist assistant started",
		"port", cfg.Port,
		"paper_mode", cfg.PaperModeOnly,
		"llm_provider", cfg.LLMProvider,
		"data_sources", cfg.DataSources(),
	)

	<-ctx.Done()
	slog.Info("shutting down...")
	srv.Shutdown()
}
