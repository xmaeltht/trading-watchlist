package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/xmaeltht/trading-watchlist/internal/store"
)

// triggerRun manually kicks off a scoring run for a given horizon.
// POST /api/runs/trigger/:horizon
func (s *Server) triggerRun(c *fiber.Ctx) error {
	horizon := store.Horizon(c.Params("horizon"))
	if !validHorizon(horizon) {
		return fiber.NewError(fiber.StatusBadRequest, "invalid horizon")
	}

	if s.sched == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "scheduler not available")
	}

	go func() {
		_ = s.sched.RunNow(c.Context(), horizon)
	}()

	return c.JSON(fiber.Map{
		"status":  "triggered",
		"horizon": horizon,
		"note":    "Scoring run started in background. Check /api/watchlist/" + string(horizon) + " in a few minutes.",
	})
}
