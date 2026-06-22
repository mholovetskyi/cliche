// Package budget implements the Budget Kernel: deterministic, pre-emptive
// spend caps that the model can never argue its way past.
//
// The token cap is the HARD guarantee (provider-independent); the dollar cap
// is a conservative ESTIMATE derived from the pricing table. Enforcement
// happens at two gates:
//
//   - Preflight: before a turn fires, using a conservative estimate.
//   - Record:    immediately after a turn, using ACTUAL usage. This catches
//     the fat-completion turn that blew the pre-flight estimate, and halts
//     before the next turn can fire.
//
// To keep a single turn from overshooting, the agent additionally bounds each
// provider request by the remaining token budget (see agent.Run). True
// token-by-token stream-abort mid-completion is roadmapped, not in v0.
package budget

import (
	"errors"
	"fmt"

	"github.com/mholovetskyi/cliche/internal/pricing"
)

// Limits define the ceilings for a run. A zero value means "no limit" for
// that dimension.
type Limits struct {
	MaxTokens int     // hard cap on total (input+output) tokens
	MaxUSD    float64 // estimated-dollar cap
}

// Usage is the running tally for a session.
type Usage struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	USD          float64 `json:"usd"`
}

// TotalTokens returns input+output tokens consumed so far.
func (u Usage) TotalTokens() int { return u.InputTokens + u.OutputTokens }

// Sentinel errors for the two cap types. Callers can errors.Is these.
var (
	ErrTokenCap = errors.New("token cap reached")
	ErrUSDCap   = errors.New("estimated dollar cap reached")
)

// Kernel enforces spend limits. It is part of the deterministic core: no LLM
// is ever in this path.
type Kernel struct {
	limits Limits
	usage  Usage
}

// New returns a Budget Kernel with the given limits.
func New(l Limits) *Kernel { return &Kernel{limits: l} }

// Usage returns the current running tally.
func (k *Kernel) Usage() Usage { return k.usage }

// Limits returns the configured limits.
func (k *Kernel) Limits() Limits { return k.limits }

// Preflight checks, BEFORE a turn fires, whether an estimated number of
// input/output tokens for model would breach a cap. It does not mutate usage.
func (k *Kernel) Preflight(model string, estInputTokens, estOutputTokens int) error {
	price, _ := pricing.Lookup(model)
	projTokens := k.usage.TotalTokens() + estInputTokens + estOutputTokens
	projUSD := k.usage.USD + price.CostUSD(estInputTokens, estOutputTokens)
	return k.check(projTokens, projUSD, "preflight")
}

// Record adds ACTUAL usage after a turn and returns an error if a cap has now
// been crossed. This is the mid-stream / post-turn gate.
func (k *Kernel) Record(model string, inputTokens, outputTokens int) error {
	price, _ := pricing.Lookup(model)
	k.usage.InputTokens += inputTokens
	k.usage.OutputTokens += outputTokens
	k.usage.USD += price.CostUSD(inputTokens, outputTokens)
	return k.check(k.usage.TotalTokens(), k.usage.USD, "recorded")
}

func (k *Kernel) check(tokens int, usd float64, stage string) error {
	if k.limits.MaxTokens > 0 && tokens >= k.limits.MaxTokens {
		return fmt.Errorf("%w (%s): %d/%d tokens", ErrTokenCap, stage, tokens, k.limits.MaxTokens)
	}
	if k.limits.MaxUSD > 0 && usd >= k.limits.MaxUSD {
		return fmt.Errorf("%w (%s): ~$%.4f/$%.2f", ErrUSDCap, stage, usd, k.limits.MaxUSD)
	}
	return nil
}

// Remaining reports how much budget is left. For USD this is best-effort
// (an estimate). Zero values mean "no limit configured" for that dimension.
func (k *Kernel) Remaining() (tokens int, usd float64) {
	if k.limits.MaxTokens > 0 {
		if tokens = k.limits.MaxTokens - k.usage.TotalTokens(); tokens < 0 {
			tokens = 0
		}
	}
	if k.limits.MaxUSD > 0 {
		if usd = k.limits.MaxUSD - k.usage.USD; usd < 0 {
			usd = 0
		}
	}
	return tokens, usd
}
