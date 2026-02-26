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
	"github.com/xmaeltht/trading-watchlist/internal/ingestor"
	"github.com/xmaeltht/trading-watchlist/internal/scheduler"
	"github.com/xmaeltht/trading-watchlist/internal/store"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg := config.Load()

	// Graceful shutdown context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Connect to PostgreSQL
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Initialize store and run migrations
	s := store.New(db)
	if err := s.Migrate(ctx); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrated")

	// Initialize data ingestors
	priceIng := ingestor.NewPriceIngestor(cfg.PolygonAPIKey, s)
	newsIng := ingestor.NewNewsIngestor(cfg.FinnhubAPIKey, s)

	// Start scheduler
	sched := scheduler.New(cfg, s, priceIng, newsIng)
	sched.Start()
	defer sched.Stop()

	// Start API server
	srv := api.NewServer(cfg, s)
	go func() {
		if err := srv.Listen(":" + cfg.Port); err != nil {
			slog.Error("server error", "error", err)
		}
	}()

	slog.Info("trading watchlist assistant started",
		"port", cfg.Port,
		"paper_mode", cfg.PaperModeOnly,
		"data_sources", cfg.DataSources(),
	)

	// Wait for shutdown signal
	<-ctx.Done()
	slog.Info("shutting down...")
	if err := srv.Shutdown(); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
	slog.Info("goodbye")
}
