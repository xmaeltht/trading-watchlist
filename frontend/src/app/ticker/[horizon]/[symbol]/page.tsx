import { api, type Horizon, riskColor, scoreColor, parseFlags, parseDataGaps } from "@/lib/api";
import ScoreBar from "@/components/ScoreBar";
import Link from "next/link";

interface Props {
  params: Promise<{ horizon: Horizon; symbol: string }>;
}

const horizonWeights: Record<Horizon, Record<string, string>> = {
  daily:   { Momentum: "30%", Volatility: "20%", Liquidity: "15%", Catalyst: "20%", Fundamentals: "5%" },
  weekly:  { Momentum: "25%", Volatility: "15%", Liquidity: "10%", Catalyst: "20%", Fundamentals: "15%" },
  monthly: { Momentum: "15%", Volatility: "10%", Liquidity: "5%",  Catalyst: "15%", Fundamentals: "35%" },
};

export default async function TickerDetail({ params }: Props) {
  const { horizon, symbol } = await params;
  let ticker;
  try {
    const res = await api.ticker(horizon, symbol.toUpperCase());
    ticker = res.ticker;
  } catch {
    return (
      <div className="text-center py-20 text-gray-500">
        <p className="text-lg">Ticker not found in {horizon} watchlist.</p>
        <Link href="/" className="text-blue-400 text-sm mt-2 inline-block hover:underline">← Back to dashboard</Link>
      </div>
    );
  }

  const flags = parseFlags(ticker.flags);
  const gaps = parseDataGaps(ticker.data_gaps);
  const weights = horizonWeights[horizon];

  return (
    <div className="space-y-6 max-w-4xl mx-auto">
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-gray-500">
        <Link href="/" className="hover:text-white transition">Dashboard</Link>
        <span>/</span>
        <span className="capitalize">{horizon}</span>
        <span>/</span>
        <span className="text-white font-bold">{ticker.ticker}</span>
      </div>

      {/* Header */}
      <div className="flex items-start justify-between flex-wrap gap-4">
        <div>
          <h1 className="text-3xl font-bold">{ticker.ticker}</h1>
          <p className="text-gray-400">{ticker.company_name} · {ticker.sector}</p>
        </div>
        <div className="flex items-center gap-3">
          <span className={`text-xs px-3 py-1.5 rounded font-medium ${riskColor(ticker.risk_rating)}`}>
            {ticker.risk_rating} RISK
          </span>
          <span className="text-xs bg-gray-800 px-3 py-1.5 rounded">
            Rank #{ticker.rank} · {horizon.toUpperCase()}
          </span>
        </div>
      </div>

      {/* Flags */}
      {flags.length > 0 && (
        <div className="flex gap-2 flex-wrap">
          {flags.map((f, i) => (
            <span key={i} className="text-sm bg-yellow-500/10 border border-yellow-600/30 text-yellow-300 px-3 py-1 rounded">
              {f}
            </span>
          ))}
        </div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        {/* Composite score card */}
        <div className="md:col-span-1 bg-gray-900 rounded-xl border border-gray-800 p-5 space-y-4">
          <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider">Composite Score</h2>
          <div className={`text-6xl font-black ${scoreColor(ticker.composite_score)}`}>
            {ticker.composite_score.toFixed(0)}
          </div>
          <div className="text-xs text-gray-500">Confidence: {ticker.confidence_score.toFixed(0)}%</div>

          <div className="space-y-3 pt-2">
            <ScoreBar label="Momentum"    score={ticker.momentum_score}    weight={weights.Momentum} />
            <ScoreBar label="Volatility"  score={ticker.volatility_score}  weight={weights.Volatility} />
            <ScoreBar label="Liquidity"   score={ticker.liquidity_score}   weight={weights.Liquidity} />
            <ScoreBar label="Catalyst"    score={ticker.catalyst_score}    weight={weights.Catalyst} />
            <ScoreBar label="Fundamentals" score={ticker.fundamental_score} weight={weights.Fundamentals} />
          </div>

          {ticker.risk_penalty > 0 && (
            <div className="text-xs text-red-400 bg-red-400/10 rounded px-2 py-1">
              Risk penalty: -{ticker.risk_penalty.toFixed(0)} pts
            </div>
          )}

          {gaps.length > 0 && (
            <div className="text-xs text-yellow-400/80 border border-yellow-600/20 bg-yellow-900/10 rounded px-2 py-1 space-y-0.5">
              <div className="font-medium">Data gaps:</div>
              {gaps.map((g, i) => <div key={i}>· {g.replace(/_/g, " ")}</div>)}
            </div>
          )}
        </div>

        {/* Thesis + Trade Plan */}
        <div className="md:col-span-2 space-y-4">
          {/* Thesis */}
          <div className="bg-gray-900 rounded-xl border border-gray-800 p-5">
            <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">Thesis</h2>
            {ticker.thesis ? (
              <div className="text-sm text-gray-200 whitespace-pre-line leading-relaxed">{ticker.thesis}</div>
            ) : (
              <p className="text-gray-500 text-sm italic">Thesis pending LLM generation.</p>
            )}
          </div>

          {/* Trade Plan */}
          <div className="bg-gray-900 rounded-xl border border-gray-800 p-5">
            <div className="flex items-center gap-2 mb-3">
              <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider">Trade Plan Template</h2>
              <span className="text-xs bg-yellow-500/20 text-yellow-400 px-2 py-0.5 rounded">NOT ADVICE</span>
            </div>
            {ticker.trade_plan_text ? (
              <div className="text-sm text-gray-200 whitespace-pre-line leading-relaxed">{ticker.trade_plan_text}</div>
            ) : (
              <p className="text-gray-500 text-sm italic">Trade plan pending generation.</p>
            )}
          </div>

          {/* Invalidation */}
          <div className="bg-gray-900 rounded-xl border border-red-900/30 p-5">
            <h2 className="text-sm font-semibold text-red-400/80 uppercase tracking-wider mb-3">
              🚫 Invalidation Criteria
            </h2>
            {ticker.invalidation_text ? (
              <div className="text-sm text-gray-300 whitespace-pre-line leading-relaxed">{ticker.invalidation_text}</div>
            ) : (
              <p className="text-gray-500 text-sm italic">Invalidation criteria pending.</p>
            )}
          </div>
        </div>
      </div>

      {/* Disclaimer */}
      <div className="text-xs text-gray-600 border border-gray-800 rounded-lg p-4 leading-relaxed">
        ⚠️ <strong>Disclaimer:</strong> This page is for informational and educational purposes only.
        It does not constitute financial advice or a recommendation to buy or sell any security.
        All investing involves risk, including possible loss of principal.
        The trade plan above is a template concept only — not a guarantee of any outcome.
      </div>
    </div>
  );
}
