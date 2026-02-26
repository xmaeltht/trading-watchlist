const BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080/api";

export type Horizon = "daily" | "weekly" | "monthly";

export interface TickerScore {
  id: number;
  run_id: string;
  horizon: Horizon;
  ticker: string;
  company_name: string;
  sector: string;
  rank: number;
  composite_score: number;
  momentum_score: number;
  volatility_score: number;
  liquidity_score: number;
  catalyst_score: number;
  fundamental_score: number;
  risk_penalty: number;
  confidence_score: number;
  data_gaps: string; // JSON array
  thesis: string;
  trade_plan_text: string;
  invalidation_text: string;
  risk_rating: "LOW" | "MEDIUM" | "HIGH";
  flags: string; // JSON array
  current_price: number;
  target_price: number;
  stop_price: number;
  rr_ratio: number;
  upside_pct: number;
  technical_snapshot: string;
  fundamental_snapshot: string;
  news_summary: string;
  created_at: string;
}

export interface WatchlistResponse {
  horizon: Horizon;
  count: number;
  paper_mode: boolean;
  tickers: TickerScore[];
  disclaimer: string;
}

export interface TickerResponse {
  horizon: Horizon;
  paper_mode: boolean;
  ticker: TickerScore;
  disclaimer: string;
}

export interface HealthResponse {
  status: string;
  paper_mode: boolean;
  sources: string[];
}

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`, { next: { revalidate: 300 } });
  if (!res.ok) throw new Error(`API error ${res.status}: ${path}`);
  return res.json();
}

export const api = {
  health: () => fetchJSON<HealthResponse>("/health"),
  watchlist: (horizon: Horizon, limit = 25) =>
    fetchJSON<WatchlistResponse>(`/watchlist/${horizon}?limit=${limit}`),
  ticker: (horizon: Horizon, symbol: string) =>
    fetchJSON<TickerResponse>(`/ticker/${horizon}/${symbol}`),
  triggerRun: async (horizon: Horizon) => {
    const res = await fetch(`${BASE}/runs/trigger/${horizon}`, { method: "POST" });
    return res.json();
  },
};

export function parseFlags(flags: string): string[] {
  try { return JSON.parse(flags) ?? []; } catch { return []; }
}

export function parseDataGaps(gaps: string): string[] {
  try { return JSON.parse(gaps) ?? []; } catch { return []; }
}

export function scoreColor(score: number): string {
  if (score >= 70) return "text-green-400";
  if (score >= 40) return "text-yellow-400";
  return "text-red-400";
}

export function riskColor(rating: string): string {
  switch (rating) {
    case "LOW": return "text-green-400 bg-green-400/10";
    case "HIGH": return "text-red-400 bg-red-400/10";
    default: return "text-yellow-400 bg-yellow-400/10";
  }
}
