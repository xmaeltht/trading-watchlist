"use client";

import { useState, useEffect } from "react";
import { api, type Horizon, type WatchlistResponse } from "@/lib/api";
import WatchlistTable from "@/components/WatchlistTable";
import RegimeBadge from "@/components/RegimeBadge";

const HORIZONS: { id: Horizon; label: string; desc: string }[] = [
  { id: "daily",   label: "Daily",   desc: "Pre-market · 1–3 day holds" },
  { id: "weekly",  label: "Weekly",  desc: "Sunday · 5–10 day holds"    },
  { id: "monthly", label: "Monthly", desc: "Month-end · 20–45 day holds" },
];

export default function Dashboard() {
  const [activeHorizon, setActiveHorizon] = useState<Horizon>("daily");
  const [data, setData] = useState<Record<Horizon, WatchlistResponse | null>>({
    daily: null, weekly: null, monthly: null,
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [triggering, setTriggering] = useState(false);

  useEffect(() => {
    if (data[activeHorizon]) return;
    setLoading(true);
    setError(null);
    api.watchlist(activeHorizon)
      .then((res) => setData((prev) => ({ ...prev, [activeHorizon]: res })))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [activeHorizon]);

  const handleTrigger = async () => {
    setTriggering(true);
    await api.triggerRun(activeHorizon);
    // Invalidate cache for this horizon
    setData((prev) => ({ ...prev, [activeHorizon]: null }));
    setTriggering(false);
  };

  const current = data[activeHorizon];

  return (
    <div className="space-y-6">
      {/* Top bar */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Watchlist</h1>
          {current && (
            <p className="text-xs text-gray-500 mt-0.5">
              Last refreshed: {new Date(current.tickers?.[0]?.created_at ?? "").toLocaleString()}
            </p>
          )}
        </div>
        <div className="flex items-center gap-3">
          <RegimeBadge />
          <button
            onClick={handleTrigger}
            disabled={triggering}
            className="text-xs bg-blue-600 hover:bg-blue-500 disabled:opacity-50 px-3 py-1.5 rounded font-medium transition"
          >
            {triggering ? "Triggering…" : "▶ Run Now"}
          </button>
          <a
            href={`/api/export/${activeHorizon}`}
            className="text-xs bg-gray-800 hover:bg-gray-700 px-3 py-1.5 rounded font-medium transition"
          >
            ↓ CSV
          </a>
        </div>
      </div>

      {/* Horizon tabs */}
      <div className="flex gap-1 bg-gray-900 p-1 rounded-lg w-fit">
        {HORIZONS.map((h) => (
          <button
            key={h.id}
            onClick={() => setActiveHorizon(h.id)}
            className={`px-4 py-2 rounded-md text-sm font-medium transition ${
              activeHorizon === h.id
                ? "bg-blue-600 text-white"
                : "text-gray-400 hover:text-white hover:bg-gray-800"
            }`}
          >
            <span>{h.label}</span>
            <span className="ml-2 text-xs opacity-60">{h.desc}</span>
          </button>
        ))}
      </div>

      {/* Content */}
      {loading && (
        <div className="flex items-center justify-center h-64">
          <div className="animate-spin text-4xl">⟳</div>
        </div>
      )}
      {error && (
        <div className="bg-red-900/30 border border-red-700 rounded-lg p-4 text-red-300">
          <strong>Error:</strong> {error}
          {error.includes("not found") && (
            <p className="mt-1 text-sm">No data yet — click "Run Now" to generate your first watchlist.</p>
          )}
        </div>
      )}
      {!loading && !error && current && (
        <WatchlistTable data={current} horizon={activeHorizon} />
      )}
      {!loading && !error && !current && !error && (
        <div className="text-center py-20 text-gray-500">
          <p className="text-lg">No watchlist data yet.</p>
          <p className="text-sm mt-2">Click <strong>Run Now</strong> to trigger your first scoring run.</p>
        </div>
      )}
    </div>
  );
}
