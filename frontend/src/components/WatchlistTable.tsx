"use client";

import Link from "next/link";
import { type WatchlistResponse, type Horizon, scoreColor, riskColor, parseFlags } from "@/lib/api";

interface Props {
  data: WatchlistResponse;
  horizon: Horizon;
}

export default function WatchlistTable({ data, horizon }: Props) {
  if (!data.tickers?.length) {
    return <p className="text-gray-500 text-center py-10">No tickers in this watchlist yet.</p>;
  }

  return (
    <div className="space-y-3">
      <p className="text-xs text-gray-500">{data.count} tickers · {data.disclaimer}</p>
      <div className="overflow-x-auto rounded-xl border border-gray-800">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-gray-800 bg-gray-900/50">
              <th className="text-left px-4 py-3 text-gray-400 font-medium w-8">#</th>
              <th className="text-left px-4 py-3 text-gray-400 font-medium">Ticker</th>
              <th className="text-left px-4 py-3 text-gray-400 font-medium hidden md:table-cell">Sector</th>
              <th className="text-right px-4 py-3 text-gray-400 font-medium">Price</th>
              <th className="text-right px-4 py-3 text-gray-400 font-medium hidden md:table-cell">Target</th>
              <th className="text-right px-4 py-3 text-gray-400 font-medium hidden md:table-cell">R/R</th>
              <th className="text-right px-4 py-3 text-gray-400 font-medium">Score</th>
              <th className="text-right px-4 py-3 text-gray-400 font-medium hidden lg:table-cell">Momentum</th>
              <th className="text-right px-4 py-3 text-gray-400 font-medium hidden lg:table-cell">Catalyst</th>
              <th className="text-right px-4 py-3 text-gray-400 font-medium hidden lg:table-cell">Fundamental</th>
              <th className="text-right px-4 py-3 text-gray-400 font-medium">Risk</th>
              <th className="text-right px-4 py-3 text-gray-400 font-medium hidden md:table-cell">Confidence</th>
              <th className="px-4 py-3 text-gray-400 font-medium">Flags</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-800/60">
            {data.tickers.map((t) => {
              const flags = parseFlags(t.flags);
              return (
                <tr
                  key={t.ticker}
                  className="hover:bg-gray-900/60 transition cursor-pointer group"
                >
                  <td className="px-4 py-3 text-gray-500 font-mono text-xs">{t.rank}</td>
                  <td className="px-4 py-3">
                    <Link href={`/ticker/${horizon}/${t.ticker}`} className="group-hover:text-blue-400 transition">
                      <span className="font-bold text-white">{t.ticker}</span>
                      <span className="ml-2 text-gray-400 text-xs hidden sm:inline truncate max-w-32">{t.company_name}</span>
                    </Link>
                  </td>
                  <td className="px-4 py-3 text-gray-400 text-xs hidden md:table-cell">{t.sector ?? "—"}</td>
                  <td className="px-4 py-3 text-right text-sm text-gray-200">${t.current_price?.toFixed(2)}</td>
                  <td className="px-4 py-3 text-right text-sm text-green-300 hidden md:table-cell">${t.target_price?.toFixed(2)}</td>
                  <td className={`px-4 py-3 text-right text-xs hidden md:table-cell ${t.rr_ratio >= 2 ? "text-green-400" : "text-yellow-400"}`}>{t.rr_ratio?.toFixed(2)}x</td>
                  <td className="px-4 py-3 text-right">
                    <span className={`font-bold text-base ${scoreColor(t.composite_score)}`}>
                      {t.composite_score.toFixed(0)}
                    </span>
                  </td>
                  <td className={`px-4 py-3 text-right text-xs hidden lg:table-cell ${scoreColor(t.momentum_score)}`}>
                    {t.momentum_score.toFixed(0)}
                  </td>
                  <td className={`px-4 py-3 text-right text-xs hidden lg:table-cell ${scoreColor(t.catalyst_score)}`}>
                    {t.catalyst_score.toFixed(0)}
                  </td>
                  <td className={`px-4 py-3 text-right text-xs hidden lg:table-cell ${scoreColor(t.fundamental_score)}`}>
                    {t.fundamental_score.toFixed(0)}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <span className={`text-xs px-2 py-0.5 rounded font-medium ${riskColor(t.risk_rating)}`}>
                      {t.risk_rating}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right text-xs text-gray-400 hidden md:table-cell">
                    {t.confidence_score.toFixed(0)}%
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex gap-1 flex-wrap">
                      {flags.slice(0, 2).map((f, i) => (
                        <span key={i} className="text-xs bg-gray-800 px-1.5 py-0.5 rounded">{f}</span>
                      ))}
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
