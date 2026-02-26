package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/xmaeltht/trading-watchlist/internal/config"
	"github.com/xmaeltht/trading-watchlist/internal/scheduler"
	"github.com/xmaeltht/trading-watchlist/internal/store"
)



type Server struct {
	app   *fiber.App
	cfg   *config.Config
	store *store.Store
	sched *scheduler.Scheduler
}

func NewServer(cfg *config.Config, s *store.Store, sched *scheduler.Scheduler) *Server {
	app := fiber.New(fiber.Config{
		AppName:      "Trading Watchlist API",
		ErrorHandler: customErrorHandler,
	})

	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{AllowOrigins: "*", AllowMethods: "GET,POST,OPTIONS"}))

	srv := &Server{app: app, cfg: cfg, store: s, sched: sched}
	srv.registerRoutes()
	return srv
}

func (s *Server) registerRoutes() {
	api := s.app.Group("/api")

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":     "ok",
			"paper_mode": s.cfg.PaperModeOnly,
			"sources":    s.cfg.DataSources(),
		})
	})

	// Watchlists
	api.Get("/watchlist/:horizon", s.getWatchlist)
	api.Get("/ticker/:horizon/:symbol", s.getTicker)
	api.Get("/export/:horizon", s.exportCSV)

	// Runs
	api.Get("/runs", s.listRuns)
	api.Post("/runs/trigger/:horizon", s.triggerRun)
}

func (s *Server) Listen(addr string) error  { return s.app.Listen(addr) }
func (s *Server) Shutdown() error           { return s.app.Shutdown() }

func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{
		"error":      err.Error(),
		"disclaimer": "This application provides decision-support only, not financial advice. All trading carries risk.",
	})
}
