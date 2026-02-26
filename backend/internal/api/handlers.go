package api

import (
	"encoding/csv"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/xmaeltht/trading-watchlist/internal/store"
)

// getWatchlist returns the latest Top-N ranked tickers for a horizon.
// GET /api/watchlist/:horizon
func (s *Server) getWatchlist(c *fiber.Ctx) error {
	horizon := store.Horizon(c.Params("horizon"))
	if !validHorizon(horizon) {
		return fiber.NewError(fiber.StatusBadRequest, "invalid horizon; use daily, weekly, or monthly")
	}

	limit := c.QueryInt("limit", s.cfg.ListSize)
	if limit > 100 {
		limit = 100
	}

	scores, err := s.store.GetWatchlist(c.Context(), horizon, limit)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to fetch watchlist: "+err.Error())
	}

	return c.JSON(fiber.Map{
		"horizon":    horizon,
		"count":      len(scores),
		"paper_mode": s.cfg.PaperModeOnly,
		"tickers":    scores,
		"disclaimer": "Decision-support only. Not financial advice. All trading carries risk.",
	})
}

// getTicker returns the latest score detail for a single ticker on a horizon.
// GET /api/ticker/:horizon/:symbol
func (s *Server) getTicker(c *fiber.Ctx) error {
	horizon := store.Horizon(c.Params("horizon"))
	if !validHorizon(horizon) {
		return fiber.NewError(fiber.StatusBadRequest, "invalid horizon")
	}

	symbol := c.Params("symbol")
	if symbol == "" {
		return fiber.NewError(fiber.StatusBadRequest, "symbol is required")
	}

	score, err := s.store.GetTicker(c.Context(), horizon, symbol)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "ticker not found for this horizon")
	}

	return c.JSON(fiber.Map{
		"horizon":    horizon,
		"paper_mode": s.cfg.PaperModeOnly,
		"ticker":     score,
		"disclaimer": "Decision-support only. Not financial advice.",
	})
}

// listRuns returns recent scoring runs.
// GET /api/runs
func (s *Server) listRuns(c *fiber.Ctx) error {
	// TODO: implement store.ListRuns
	return c.JSON(fiber.Map{"runs": []string{}, "note": "not yet implemented"})
}

// exportCSV exports the latest watchlist as a CSV file.
// GET /api/export/:horizon
func (s *Server) exportCSV(c *fiber.Ctx) error {
	horizon := store.Horizon(c.Params("horizon"))
	if !validHorizon(horizon) {
		return fiber.NewError(fiber.StatusBadRequest, "invalid horizon")
	}

	scores, err := s.store.GetWatchlist(c.Context(), horizon, s.cfg.ListSize)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to fetch watchlist")
	}

	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=watchlist_%s.csv", horizon))

	w := csv.NewWriter(c)
	defer w.Flush()

	// Header
	w.Write([]string{
		"Rank", "Ticker", "Company", "Sector", "Score",
		"Momentum", "Volatility", "Liquidity", "Catalyst", "Fundamentals",
		"Risk Penalty", "Risk Rating", "Confidence",
	})

	for _, sc := range scores {
		w.Write([]string{
			fmt.Sprintf("%d", sc.Rank),
			sc.Ticker,
			sc.CompanyName,
			sc.Sector,
			fmt.Sprintf("%.1f", sc.CompositeScore),
			fmt.Sprintf("%.1f", sc.MomentumScore),
			fmt.Sprintf("%.1f", sc.VolatilityScore),
			fmt.Sprintf("%.1f", sc.LiquidityScore),
			fmt.Sprintf("%.1f", sc.CatalystScore),
			fmt.Sprintf("%.1f", sc.FundamentalScore),
			fmt.Sprintf("%.1f", sc.RiskPenalty),
			sc.RiskRating,
			fmt.Sprintf("%.0f%%", sc.ConfidenceScore),
		})
	}

	return nil
}

func validHorizon(h store.Horizon) bool {
	return h == store.HorizonDaily || h == store.HorizonWeekly || h == store.HorizonMonthly
}
