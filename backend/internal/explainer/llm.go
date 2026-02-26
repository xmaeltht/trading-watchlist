package explainer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/xmaeltht/trading-watchlist/internal/scorer"
	"github.com/xmaeltht/trading-watchlist/internal/store"
)

// Explainer generates human-readable thesis and trade plan text for a scored ticker.
type Explainer struct {
	provider string // "ollama" | "anthropic"
	model    string
	baseURL  string // for ollama
	apiKey   string // for anthropic
	client   *http.Client
}

func New(provider, model, baseURL, apiKey string) *Explainer {
	return &Explainer{
		provider: provider,
		model:    model,
		baseURL:  strings.TrimRight(baseURL, "/"),
		apiKey:   apiKey,
		client:   &http.Client{Timeout: 90 * time.Second},
	}
}

// ExplainerInput bundles everything the LLM needs to write a thesis.
type ExplainerInput struct {
	Result    scorer.ScoreResult
	Input     scorer.TickerInput
	Horizon   store.Horizon
	Rank      int
	RankAbove *scorer.ScoreResult // the ticker ranked just above, for comparison
}

// ExplainerOutput holds the LLM-generated text fields.
type ExplainerOutput struct {
	Thesis           string
	TradePlanText    string
	InvalidationText string
}

// Explain generates thesis, trade plan, and invalidation for one ticker.
// Results are cached by caller — this does a fresh LLM call each time.
func (e *Explainer) Explain(ctx context.Context, in ExplainerInput) (ExplainerOutput, error) {
	if e.provider == "" {
		return fallbackExplainer(in), nil
	}

	prompt := buildPrompt(in)
	text, err := e.callLLM(ctx, prompt)
	if err != nil {
		slog.Warn("LLM explainer failed, using fallback", "ticker", in.Result.Ticker, "error", err)
		return fallbackExplainer(in), nil
	}

	return parseOutput(text, in), nil
}

// ExplainBatch generates explanations for a ranked list, respecting context limits.
func (e *Explainer) ExplainBatch(ctx context.Context, results []scorer.ScoreResult, inputs map[string]scorer.TickerInput, horizon store.Horizon) map[string]ExplainerOutput {
	out := make(map[string]ExplainerOutput, len(results))
	for i, r := range results {
		inp := inputs[r.Ticker]
		var above *scorer.ScoreResult
		if i > 0 {
			above = &results[i-1]
		}
		o, err := e.Explain(ctx, ExplainerInput{
			Result:    r,
			Input:     inp,
			Horizon:   horizon,
			Rank:      i + 1,
			RankAbove: above,
		})
		if err != nil {
			slog.Warn("explainer error", "ticker", r.Ticker, "error", err)
			o = fallbackExplainer(ExplainerInput{Result: r, Input: inp, Horizon: horizon, Rank: i + 1})
		}
		out[r.Ticker] = o
	}
	return out
}

// ─── Prompt builder ──────────────────────────────────────────────────────────

func buildPrompt(in ExplainerInput) string {
	r := in.Result
	t := in.Input
	horizonDays := map[store.Horizon]string{
		store.HorizonDaily:   "1–3 days",
		store.HorizonWeekly:  "5–10 days",
		store.HorizonMonthly: "20–45 days",
	}[in.Horizon]

	var riskComparison string
	if in.RankAbove != nil {
		riskComparison = fmt.Sprintf(
			"The ticker ranked above (%s, score %.0f) leads on: see sub-scores.",
			in.RankAbove.Ticker, in.RankAbove.CompositeScore,
		)
	}

	data, _ := json.MarshalIndent(map[string]interface{}{
		"ticker":           r.Ticker,
		"company":          r.CompanyName,
		"sector":           r.Sector,
		"horizon":          string(in.Horizon),
		"holding_window":   horizonDays,
		"rank":             in.Rank,
		"composite_score":  r.CompositeScore,
		"momentum_score":   r.MomentumScore,
		"volatility_score": r.VolatilityScore,
		"liquidity_score":  r.LiquidityScore,
		"catalyst_score":   r.CatalystScore,
		"fundamental_score": r.FundamentalScore,
		"risk_penalty":     r.RiskPenalty,
		"risk_rating":      r.RiskRating,
		"confidence":       r.ConfidenceScore,
		"data_gaps":        r.DataGaps,
		"flags":            r.Flags,
		"price":            t.Price,
		"rsi14":            t.RSI14,
		"atr_pct":          t.ATR14Pct,
		"roc10":            t.ROC10,
		"avg_vol_30d":      t.AvgVol30d,
		"earnings_days":    t.EarningsDaysAway,
		"news_sentiment":   t.NewsSentiment7d,
		"news_count_7d":    t.NewsCount7d,
		"iv_rank":          t.IVRank,
		"rank_context":     riskComparison,
	}, "", "  ")

	return fmt.Sprintf(`You are a senior quant analyst writing a concise, evidence-based trade brief.
You do NOT promise returns or give financial advice. You provide decision-support.

Given the following scored ticker data, produce exactly this JSON output (no extra text):
{
  "thesis": "3-6 bullet points (each prefixed with •) explaining WHY this ticker is on the list, grounded in the data provided. Be specific about signals, not vague.",
  "trade_plan": "3-4 bullet points covering: entry zone concept, stop concept, profit target concept, and time stop. Use phrases like 'concept' or 'template' — never 'guaranteed'. Align to the horizon holding window.",
  "invalidation": "2-3 bullet points: what signals would make this thesis wrong and require exit."
}

TICKER DATA:
%s

Important rules:
- Ground every bullet in the actual data provided (scores, indicators, flags)
- Mention specific numbers (RSI, ATR%%, score values) where relevant
- Never claim guaranteed returns or exact price targets
- If data_gaps exist, acknowledge missing data briefly in the confidence note
- Keep language clear and direct — no jargon overload`, string(data))
}

// ─── LLM call ────────────────────────────────────────────────────────────────

func (e *Explainer) callLLM(ctx context.Context, prompt string) (string, error) {
	switch e.provider {
	case "ollama":
		return e.callOllama(ctx, prompt)
	case "anthropic":
		return e.callAnthropic(ctx, prompt)
	default:
		return "", fmt.Errorf("unknown LLM provider: %s", e.provider)
	}
}

func (e *Explainer) callOllama(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model":  e.model,
		"prompt": prompt,
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.3,
			"num_predict": 600,
		},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama decode error: %w", err)
	}
	return result.Response, nil
}

func (e *Explainer) callAnthropic(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"model":      e.model,
		"max_tokens": 700,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", e.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("anthropic error %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("anthropic decode error: %w", err)
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("anthropic returned empty content")
	}
	return result.Content[0].Text, nil
}

// ─── Output parsing ───────────────────────────────────────────────────────────

// parseOutput extracts the thesis/trade_plan/invalidation fields from LLM JSON output.
// Falls back gracefully if the LLM returns malformed text.
func parseOutput(text string, in ExplainerInput) ExplainerOutput {
	// Try to extract JSON block even if there's surrounding text
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		text = text[start : end+1]
	}

	var parsed struct {
		Thesis       string `json:"thesis"`
		TradePlan    string `json:"trade_plan"`
		Invalidation string `json:"invalidation"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		slog.Warn("LLM output JSON parse failed, using fallback", "error", err)
		return fallbackExplainer(in)
	}

	return ExplainerOutput{
		Thesis:           parsed.Thesis,
		TradePlanText:    parsed.TradePlan,
		InvalidationText: parsed.Invalidation,
	}
}

// ─── Fallback (no LLM) ───────────────────────────────────────────────────────

// fallbackExplainer generates template-based text when the LLM is unavailable.
func fallbackExplainer(in ExplainerInput) ExplainerOutput {
	r := in.Result
	t := in.Input

	var thesisParts []string
	thesisParts = append(thesisParts, fmt.Sprintf("• Composite score %.0f/100 on %s horizon (Risk: %s)", r.CompositeScore, in.Horizon, r.RiskRating))

	if r.MomentumScore >= 60 {
		thesisParts = append(thesisParts, fmt.Sprintf("• Strong momentum score (%.0f) — RSI %.0f, 10d ROC %.1f%%", r.MomentumScore, t.RSI14, t.ROC10))
	}
	if r.CatalystScore >= 60 {
		thesisParts = append(thesisParts, fmt.Sprintf("• Elevated catalyst score (%.0f) — news sentiment %.2f, %d articles in 7 days", r.CatalystScore, t.NewsSentiment7d, t.NewsCount7d))
	}
	if r.FundamentalScore >= 60 {
		thesisParts = append(thesisParts, fmt.Sprintf("• Solid fundamental score (%.0f) — earnings growth and margins above sector", r.FundamentalScore))
	}
	if len(r.DataGaps) > 0 {
		thesisParts = append(thesisParts, fmt.Sprintf("• ⚠️ Data gaps: %s — confidence %.0f%%", strings.Join(r.DataGaps, ", "), r.ConfidenceScore))
	}

	tradePlan := fmt.Sprintf(
		"• Entry zone concept: near current price $%.2f after confirmation\n"+
			"• Stop concept: ATR-based (%.1f%% of price) below nearest support\n"+
			"• Target concept: 2–3× risk reward aligned to %s window\n"+
			"• Time stop: exit if thesis not moving within horizon",
		t.Price, t.ATR14Pct, in.Horizon,
	)

	invalidation := fmt.Sprintf(
		"• Price closes below key support or 200-day MA\n"+
			"• RSI breaks below 40 with expanding volume\n"+
			"• News sentiment flips negative or earnings disappoint",
	)

	return ExplainerOutput{
		Thesis:           strings.Join(thesisParts, "\n"),
		TradePlanText:    tradePlan,
		InvalidationText: invalidation,
	}
}
