package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server
	Port      string
	JWTSecret string

	// Database
	DatabaseURL string

	// Redis
	RedisURL string

	// Data API keys
	PolygonAPIKey      string // OHLCV, corporate actions
	AlphaVantageAPIKey string // Fundamentals, EPS estimates
	FinnhubAPIKey      string // News, earnings calendar, analyst ratings
	NewsAPIKey         string // Supplementary headlines

	// LLM for thesis generation
	OllamaBaseURL   string
	AnthropicAPIKey string
	LLMProvider     string // "ollama" | "anthropic"
	LLMModel        string

	// Scoring universe
	UniverseList string // comma-separated extra tickers; default = SP500+NDX+RUT1000
	MaxPerSector int    // max tickers per GICS sector in final list (default 5)
	ListSize     int    // tickers per horizon (default 25)

	// Risk
	VIXTicker     string // default "VIX"
	PaperModeOnly bool   // if true, all output is tagged [PAPER]
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	cfg := &Config{
		Port:      getEnv("PORT", "8080"),
		JWTSecret: getEnv("JWT_SECRET", "change-me"),

		DatabaseURL: getEnv("DATABASE_URL", "postgres://watchlist:watchlist@localhost:5432/watchlist?sslmode=disable"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),

		PolygonAPIKey:      getEnv("POLYGON_API_KEY", ""),
		AlphaVantageAPIKey: getEnv("ALPHA_VANTAGE_API_KEY", ""),
		FinnhubAPIKey:      getEnv("FINNHUB_API_KEY", ""),
		NewsAPIKey:         getEnv("NEWS_API_KEY", ""),

		OllamaBaseURL:   getEnv("OLLAMA_BASE_URL", "http://localhost:11434"),
		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
		LLMProvider:     getEnv("LLM_PROVIDER", "ollama"),
		LLMModel:        getEnv("LLM_MODEL", "llama3.2"),

		UniverseList:  getEnv("UNIVERSE_LIST", ""),
		MaxPerSector:  getEnvInt("MAX_PER_SECTOR", 5),
		ListSize:      getEnvInt("LIST_SIZE", 25),
		VIXTicker:     getEnv("VIX_TICKER", "VIX"),
		PaperModeOnly: getEnvBool("PAPER_MODE_ONLY", true), // safe default
	}

	cfg.validate()
	return cfg
}

func (c *Config) validate() {
	if c.JWTSecret == "change-me" {
		log.Println("WARNING: JWT_SECRET is not set — using insecure default. Set JWT_SECRET in production.")
	}
	if c.PolygonAPIKey == "" {
		log.Println("WARNING: POLYGON_API_KEY not set — price ingestor will use demo/cached data only.")
	}
}

// DataSources returns which API providers are configured.
func (c *Config) DataSources() []string {
	var sources []string
	if c.PolygonAPIKey != "" {
		sources = append(sources, "polygon")
	}
	if c.AlphaVantageAPIKey != "" {
		sources = append(sources, "alphavantage")
	}
	if c.FinnhubAPIKey != "" {
		sources = append(sources, "finnhub")
	}
	return sources
}

// ExtraUniverseTickers parses the UNIVERSE_LIST env var.
func (c *Config) ExtraUniverseTickers() []string {
	if c.UniverseList == "" {
		return nil
	}
	parts := strings.Split(c.UniverseList, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(strings.ToUpper(p)); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "true" || v == "1" || v == "yes"
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

// Ensure getEnvFloat is used (avoids unused import warning in future).
var _ = fmt.Sprintf
var _ = getEnvFloat
