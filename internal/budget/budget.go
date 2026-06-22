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
// is ever in this path. Kernels nest: a scoped child kernel enforces its own
// (tighter) limits AND bubbles every charge up to its parent, so a subagent
// can never exceed either its own sub-budget or the shared session cap.
type Kernel struct {
	limits Limits
	usage  Usage
	parent *Kernel
}

// New returns a Budget Kernel with the given limits.
func New(l Limits) *Kernel { return &Kernel{limits: l} }

// Scoped returns a child kernel with its own limits that also charges this
// kernel (and its ancestors). Use for subagents: the child gets a bounded slice
// while the session cap on the root remains authoritative.
func (k *Kernel) Scoped(l Limits) *Kernel { return &Kernel{limits: l, parent: k} }

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
	if err := k.check(projTokens, projUSD, "preflight"); err != nil {
		return err
	}
	if k.parent != nil {
		return k.parent.Preflight(model, estInputTokens, estOutputTokens)
	}
	return nil
}

// Record adds ACTUAL usage after a turn and returns an error if a cap has now
// been crossed. This is the mid-stream / post-turn gate. The charge bubbles to
// ancestor kernels so the session cap stays authoritative across subagents.
func (k *Kernel) Record(model string, inputTokens, outputTokens int) error {
	price, _ := pricing.Lookup(model)
	k.usage.InputTokens += inputTokens
	k.usage.OutputTokens += outputTokens
	k.usage.USD += price.CostUSD(inputTokens, outputTokens)
	// Always bubble the charge so the root stays authoritative even when this
	// level's cap trips; the local cap is the more specific error and wins.
	var perr error
	if k.parent != nil {
		perr = k.parent.Record(model, inputTokens, outputTokens)
	}
	if err := k.check(k.usage.TotalTokens(), k.usage.USD, "recorded"); err != nil {
		return err
	}
	return perr
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

// Remaining reports the TIGHTEST remaining budget across this kernel and its
// ancestors (so a subagent's headroom never exceeds the session's). For USD
// this is best-effort. Zero values mean "no limit configured" anywhere.
func (k *Kernel) Remaining() (tokens int, usd float64) {
	tok, dol := -1, -1.0 // -1 == unlimited so far
	for cur := k; cur != nil; cur = cur.parent {
		if cur.limits.MaxTokens > 0 {
			r := cur.limits.MaxTokens - cur.usage.TotalTokens()
			if r < 0 {
				r = 0
			}
			if tok == -1 || r < tok {
				tok = r
			}
		}
		if cur.limits.MaxUSD > 0 {
			r := cur.limits.MaxUSD - cur.usage.USD
			if r < 0 {
				r = 0
			}
			if dol == -1 || r < dol {
				dol = r
			}
		}
	}
	if tok == -1 {
		tok = 0
	}
	if dol == -1 {
		dol = 0
	}
	return tok, dol
}
