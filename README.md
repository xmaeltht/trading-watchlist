# Trading Watchlist Assistant

A self-hosted, data-driven decision-support tool that surfaces the **Top 25 most tradeable stocks** per horizon (Daily / Weekly / Monthly), explains why each made the list, and provides structured trade plan templates — all with hard risk guardrails.

> ⚠️ **This is decision-support, not financial advice.** All trading carries risk, including possible loss of principal. Past performance does not guarantee future results. No claims of guaranteed returns.

## Features

- **Multi-horizon watchlists** — Daily (pre-market), Weekly (Sunday), Monthly
- **Transparent scoring** — 6-component model: Momentum, Volatility, Liquidity, Catalyst, Fundamentals, Risk
- **Explainable rankings** — every pick includes thesis bullets, confidence score, and data gaps
- **Risk guardrails** — sector concentration limits, earnings flags, IV warnings, regime detection
- **LLM-powered briefs** — human-readable thesis generation via Ollama or Anthropic
- **Paper trading mode** — safe default, all output tagged `[PAPER]`
- **CSV export** — one-click download of watchlists
- **Scheduled refreshes** — cron-based scoring runs aligned to each horizon

## Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.24 + Fiber v2 |
| Database | PostgreSQL 16 |
| Cache | Redis 7 |
| Data APIs | Polygon.io, Finnhub, Alpha Vantage |
| LLM | Ollama (local) or Anthropic |
| Container | Podman / Docker |
| Orchestration | Kubernetes + Helm |

## Quick Start

```bash
# 1. Clone
git clone https://github.com/xmaeltht/trading-watchlist.git
cd trading-watchlist

# 2. Configure
cp .env.example .env
# Edit .env — add your API keys (Polygon, Finnhub at minimum)

# 3. Start
podman compose up --build

# 4. API
curl http://localhost:8080/api/health
curl http://localhost:8080/api/watchlist/daily
```

## Scoring Model

Each ticker receives a Composite Score (0–100) from weighted sub-scores:

| Component | Daily | Weekly | Monthly |
|---|---|---|---|
| Momentum | 30% | 25% | 15% |
| Volatility | 20% | 15% | 10% |
| Liquidity | 15% | 10% | 5% |
| Catalyst | 20% | 20% | 15% |
| Fundamentals | 5% | 15% | 35% |
| Risk Penalty | 10% | 15% | 20% |

## Risk Controls

- Max 5 tickers per sector (`MAX_PER_SECTOR`)
- Earnings proximity flags within horizon window
- IV rank warnings (>70)
- Social/social-news spike guard (`SocialSpikeZ` or abnormal news volume)
- Momentum trap filtering (>30% move, no news)
- SPY trend-based regime gating (`BULL`/`NEUTRAL`/`BEAR`) with penalty adjustment
- Data-staleness penalties (price/news/fundamentals) surfaced in `data_gaps`

## API Endpoints

- `GET /api/health` — service health + latest run metadata by horizon
- `GET /api/watchlist/:horizon` — latest ranked watchlist for `daily|weekly|monthly`
- `GET /api/ticker/:horizon/:symbol` — latest detailed ticker record
- `GET /api/export/:horizon` — CSV export
- `GET /api/runs` — latest run per horizon
- `GET /api/runs/status/:id` — progress and status for a run id
- `GET /api/runs/latest/:horizon` — latest run for one horizon
- `POST /api/runs/trigger/:horizon` — trigger scoring run

## Notes

- Company/sector enrichment is sourced from Alpha Vantage company overview when available, with built-in fallback mapping for the default universe.
- Fundamentals are optional but supported; missing API keys degrade gracefully and are surfaced via confidence/data-gap flags.

## License

MIT
