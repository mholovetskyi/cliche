// Package pricing holds per-model token prices used to turn the hard token
// cap into an estimated dollar cap.
//
// These tables are a CONTINUOUSLY-MAINTAINED CORRECTNESS ASSET, not a
// one-time constant. Prices drift; provider billing changes. Two deliberate
// safety properties:
//
//  1. Conservative rounding. When unsure, round UP. The dollar cap should
//     never let you silently overspend because the table was stale.
//  2. Unknown-model fallback is HIGH. An unrecognized model can never produce
//     a cheap estimate that defeats the dollar cap.
//
// The token cap is the deterministic guarantee. Dollars are an estimate
// derived from this file.
package pricing

// Price is the cost of a model in USD per 1,000,000 tokens.
type Price struct {
	InputPerM  float64
	OutputPerM float64
}

// table holds illustrative default prices (USD / 1M tokens). Treat these as
// maintained defaults, not authoritative billing. Override per-model in
// config when you know your real contracted rates.
var table = map[string]Price{
	"claude-opus-4-8":   {InputPerM: 15, OutputPerM: 75},
	"claude-sonnet-4-6": {InputPerM: 3, OutputPerM: 15},
	"claude-haiku-4-5":  {InputPerM: 1, OutputPerM: 5},
	"gpt-5":             {InputPerM: 10, OutputPerM: 30},
	"o4-mini":           {InputPerM: 1.1, OutputPerM: 4.4},
	"gemini-2.5-pro":    {InputPerM: 2.5, OutputPerM: 15},
	"mock":              {InputPerM: 1, OutputPerM: 1},
	// OpenRouter-style model ids (illustrative defaults).
	"openai/gpt-4o-mini":          {InputPerM: 0.15, OutputPerM: 0.60},
	"openai/gpt-4o":               {InputPerM: 2.5, OutputPerM: 10},
	"anthropic/claude-3.5-haiku":  {InputPerM: 0.80, OutputPerM: 4},
	"anthropic/claude-3.5-sonnet": {InputPerM: 3, OutputPerM: 15},
	"google/gemini-2.0-flash-001": {InputPerM: 0.10, OutputPerM: 0.40},
}

// fallback is used for unknown models: deliberately expensive so an unknown
// model over-estimates rather than under-estimates cost.
var fallback = Price{InputPerM: 20, OutputPerM: 80}

// Lookup returns the price for a model and whether it was found in the table.
// Unknown models return the high fallback price.
func Lookup(model string) (Price, bool) {
	if p, ok := table[model]; ok {
		return p, true
	}
	return fallback, false
}

// CostUSD estimates the dollar cost of the given input/output token counts.
func (p Price) CostUSD(inputTokens, outputTokens int) float64 {
	return float64(inputTokens)/1e6*p.InputPerM + float64(outputTokens)/1e6*p.OutputPerM
}
