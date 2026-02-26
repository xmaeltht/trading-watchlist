import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Trading Watchlist Assistant",
  description: "Decision-support tool for ranked trade candidates. Not financial advice.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className="dark">
      <body className="bg-gray-950 text-gray-100 min-h-screen antialiased">
        <header className="border-b border-gray-800 px-6 py-3 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <span className="text-xl font-bold tracking-tight">📈 Watchlist Assistant</span>
            <span className="text-xs bg-yellow-500/20 text-yellow-400 px-2 py-0.5 rounded font-mono">PAPER MODE</span>
          </div>
          <span className="text-xs text-gray-500">Decision-support only · Not financial advice</span>
        </header>
        <main className="max-w-7xl mx-auto px-4 py-6">{children}</main>
        <footer className="border-t border-gray-800 px-6 py-4 text-center text-xs text-gray-600">
          ⚠️ This tool provides decision-support only and does not constitute financial advice.
          All trading carries risk, including possible loss of principal.
          Past performance does not guarantee future results.
        </footer>
      </body>
    </html>
  );
}
