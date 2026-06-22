// Package agent is the loop that wraps a model in the Trust Kernel. Every
// turn passes through the Governor (loop breakers) and the Budget Kernel
// (spend caps) before and after the model runs. Halts are always structured
// and recorded to the ledger.
package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/mholovetskyi/cliche/internal/budget"
	"github.com/mholovetskyi/cliche/internal/governor"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/pricing"
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/tools"
)

// Config holds per-run agent settings.
type Config struct {
	System string
	Model  string
	// Preflight token estimates used by the Budget Kernel's first gate.
	EstInputTokens  int
	EstOutputTokens int
	// MaxWallClock, when > 0, bounds the whole run (including any single tool
	// command) so a hung shell command cannot outlast the wall-clock breaker.
	MaxWallClock time.Duration
}

// Agent ties the Trust Kernel around a provider and tool executor.
type Agent struct {
	prov provider.Provider
	bud  *budget.Kernel
	gov  *governor.Governor
	led  *ledger.Ledger
	exec tools.Executor
	cfg  Config
}

// New builds an Agent. EstInputTokens/EstOutputTokens default to conservative
// values if unset.
func New(p provider.Provider, b *budget.Kernel, g *governor.Governor, l *ledger.Ledger, e tools.Executor, cfg Config) *Agent {
	if cfg.EstInputTokens <= 0 {
		cfg.EstInputTokens = 5000
	}
	if cfg.EstOutputTokens <= 0 {
		cfg.EstOutputTokens = 1000
	}
	return &Agent{prov: p, bud: b, gov: g, led: l, exec: e, cfg: cfg}
}

// Stop codes for an Outcome.
const (
	StopCompleted = "completed"
	StopBudget    = "budget"
	StopError     = "error"
)

// Outcome is the structured result of a run.
type Outcome struct {
	Stop   string       `json:"stop"`   // "completed" | "budget" | "error" | a governor halt code
	Reason string       `json:"reason"` // human-readable detail
	Turns  int          `json:"turns"`
	Usage  budget.Usage `json:"usage"`
}

// Run drives the loop until the model completes, a breaker trips, or a cap is
// hit. It never returns nil,nil silently: every exit path is a structured
// Outcome.
func (a *Agent) Run(ctx context.Context, prompt string) (Outcome, error) {
	// Bound the whole run (and any single tool command) by the wall-clock
	// limit, so a hung command cannot outlast the Governor's breaker, which is
	// only checked between turns.
	if a.cfg.MaxWallClock > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.cfg.MaxWallClock)
		defer cancel()
	}

	messages := []provider.Message{{Role: "user", Text: prompt}}

	for {
		turn, halt := a.gov.BeginTurn()
		if halt != nil {
			return a.halted(*halt), nil
		}

		// Gate 1 (preflight): conservative estimate before the model fires.
		if err := a.bud.Preflight(a.cfg.Model, a.cfg.EstInputTokens, a.cfg.EstOutputTokens); err != nil {
			return a.budgetHalt(err, turn), nil
		}

		req := provider.Request{System: a.cfg.System, Model: a.cfg.Model, Messages: messages, Tools: DefaultToolSpecs()}
		// Bound this request's output by the remaining token budget so a single
		// turn cannot overshoot the hard token cap.
		if a.bud.Limits().MaxTokens > 0 {
			if rem, _ := a.bud.Remaining(); rem > 0 {
				req.MaxOutputTokens = rem
			}
		}

		resp, err := a.prov.Complete(ctx, req)
		if err != nil {
			a.logf(turn, ledger.EventInfo, "provider error: "+err.Error())
			return Outcome{Stop: StopError, Reason: err.Error(), Turns: turn, Usage: a.bud.Usage()}, err
		}

		// Gate 2 (post-turn): record ACTUAL usage. Catches a fat-completion
		// turn that blew the pre-flight estimate; halts before the next turn.
		capErr := a.bud.Record(a.cfg.Model, resp.Usage.InputTokens, resp.Usage.OutputTokens)
		price, _ := pricing.Lookup(a.cfg.Model)
		a.led.Append(ledger.Entry{
			Turn: turn, Event: ledger.EventTurn, Model: a.cfg.Model,
			InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens,
			USD:    price.CostUSD(resp.Usage.InputTokens, resp.Usage.OutputTokens),
			Detail: truncate(resp.Text, 120),
		})
		if capErr != nil {
			return a.budgetHalt(capErr, turn), nil
		}

		// Record the assistant turn in the transcript.
		messages = append(messages, provider.Message{Role: "assistant", Text: resp.Text, ToolCalls: resp.ToolCalls})

		// No tool calls => the model produced its final answer.
		if len(resp.ToolCalls) == 0 {
			a.logf(turn, ledger.EventInfo, "completed")
			return Outcome{Stop: StopCompleted, Reason: resp.Text, Turns: turn, Usage: a.bud.Usage()}, nil
		}

		madeProgress := false
		results := make([]provider.ToolResult, 0, len(resp.ToolCalls))
		for _, call := range resp.ToolCalls {
			if h := a.gov.RecordToolCall(call.Signature); h != nil {
				return a.halted(*h), nil
			}
			res := a.exec.Execute(ctx, call.Name, call.Args)
			a.led.Append(ledger.Entry{
				Turn: turn, Event: ledger.EventTool,
				Detail: fmt.Sprintf("%s success=%t", call.Name, res.Success),
			})
			// Any successful tool call (not just an edit) counts as progress, so
			// legitimately read-only/exploratory work is not falsely halted.
			if res.Success {
				madeProgress = true
			}
			if res.IsEdit {
				if h := a.gov.RecordEdit(res.Success); h != nil {
					return a.halted(*h), nil
				}
			}
			results = append(results, provider.ToolResult{ID: call.ID, Content: res.Output, IsError: !res.Success})
		}

		// Feed tool results back to the model as the next user turn.
		messages = append(messages, provider.Message{Role: "user", ToolResults: results})

		if h := a.gov.RecordTurnProgress(madeProgress); h != nil {
			return a.halted(*h), nil
		}
	}
}

// DefaultToolSpecs are the tools advertised to the model. v0 ships read/write
// file and a shell command; the executor still gates each behind permissions.
func DefaultToolSpecs() []provider.ToolSpec {
	strProp := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	return []provider.ToolSpec{
		{
			Name:        "read_file",
			Description: "Read a UTF-8 text file and return its contents.",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"file": strProp("path to the file to read")},
				"required":   []string{"file"},
			},
		},
		{
			Name:        "write_file",
			Description: "Write (overwrite) a file with the given contents.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":    strProp("path to the file to write"),
					"content": strProp("full new contents of the file"),
				},
				"required": []string{"file", "content"},
			},
		},
		{
			Name:        "run_command",
			Description: "Run a shell command in the project directory and return its output.",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"command": strProp("the shell command to run")},
				"required":   []string{"command"},
			},
		},
	}
}

func (a *Agent) halted(h governor.HaltReason) Outcome {
	a.led.Append(ledger.Entry{Turn: h.Turn, Event: ledger.EventHalt, Detail: h.Code + ": " + h.Detail})
	return Outcome{Stop: h.Code, Reason: h.Detail, Turns: h.Turn, Usage: a.bud.Usage()}
}

func (a *Agent) budgetHalt(err error, turn int) Outcome {
	a.led.Append(ledger.Entry{Turn: turn, Event: ledger.EventHalt, Detail: "budget: " + err.Error()})
	return Outcome{Stop: StopBudget, Reason: err.Error(), Turns: turn, Usage: a.bud.Usage()}
}

func (a *Agent) logf(turn int, event, detail string) {
	a.led.Append(ledger.Entry{Turn: turn, Event: event, Detail: detail})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
