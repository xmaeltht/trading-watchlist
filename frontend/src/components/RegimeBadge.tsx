"use client";

// Displays market regime based on VIX level (fetched from backend health/regime endpoint).
// For MVP this is a static placeholder — Phase 2 wires real VIX data.
export default function RegimeBadge() {
  return (
    <span className="text-xs bg-green-500/20 text-green-400 px-2.5 py-1 rounded font-medium">
      🟢 RISK-ON
    </span>
  );
}
