// Package agent is the loop that wraps a model in the Trust Kernel. Every
// turn passes through the Governor (loop breakers) and the Budget Kernel
// (spend caps) before and after the model runs. Halts are always structured
// and recorded to the ledger.
//
// An Agent persists its conversation transcript and budget across Run calls,
// so a single Agent can drive a multi-prompt interactive session. The Budget
// Kernel is therefore session-cumulative, while a fresh Governor is created
// per Run so loop breakers (turns, repetition) are scoped to one task.
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/budget"
	"github.com/mholovetskyi/cliche/internal/governor"
	"github.com/mholovetskyi/cliche/internal/history"
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
	// ContextLimitTokens, when > 0, enables the Context Ledger: the transcript
	// is compacted (never silently) when its estimate exceeds this.
	ContextLimitTokens int
	ContextKeepRecent  int
}

// Agent ties the Trust Kernel around a provider and tool executor.
type Agent struct {
	prov      provider.Provider
	bud       *budget.Kernel
	govLimits governor.Limits
	led       *ledger.Ledger
	exec      tools.Executor
	cfg       Config
	obs       Observer
	hist      *history.Manager
	messages  []provider.Message // persists across Run calls (the session transcript)
}

// New builds an Agent. EstInputTokens/EstOutputTokens default to conservative
// values if unset. A fresh Governor is created per Run from govLimits.
func New(p provider.Provider, b *budget.Kernel, govLimits governor.Limits, l *ledger.Ledger, e tools.Executor, cfg Config) *Agent {
	if cfg.EstInputTokens <= 0 {
		cfg.EstInputTokens = 5000
	}
	if cfg.EstOutputTokens <= 0 {
		cfg.EstOutputTokens = 1000
	}
	a := &Agent{prov: p, bud: b, govLimits: govLimits, led: l, exec: e, cfg: cfg}
	if cfg.ContextLimitTokens > 0 {
		a.hist = history.New(cfg.ContextLimitTokens, cfg.ContextKeepRecent)
	}
	return a
}

// SetObserver registers a streaming observer for live activity.
func (a *Agent) SetObserver(obs Observer) { a.obs = obs }

// Usage returns the session-cumulative budget usage.
func (a *Agent) Usage() budget.Usage { return a.bud.Usage() }

// Limits returns the budget limits.
func (a *Agent) Limits() budget.Limits { return a.bud.Limits() }

// Reset clears the conversation transcript (the budget is preserved).
func (a *Agent) Reset() { a.messages = nil }

// ContextStats returns the current estimated token size of the transcript and
// how many times it has been compacted.
func (a *Agent) ContextStats() (estTokens, compactions int) {
	estTokens = history.EstimateTokens(a.messages)
	if a.hist != nil {
		compactions = a.hist.Stats().Compactions
	}
	return estTokens, compactions
}

// RecoverContext restores the transcript to its pre-compaction state, if any.
func (a *Agent) RecoverContext() bool {
	if a.hist == nil {
		return false
	}
	if full, ok := a.hist.Recover(); ok {
		a.messages = full
		return true
	}
	return false
}

// Stop codes for an Outcome.
const (
	StopCompleted = "completed"
	StopBudget    = "budget"
	StopError     = "error"
)

// Outcome is the structured result of a run.
type Outcome struct {
	Stop    string       `json:"stop"` // "completed" | "budget" | "error" | a governor halt code
	Reason  string       `json:"reason"`
	Turns   int          `json:"turns"`
	Usage   budget.Usage `json:"usage"`
	Verdict string       `json:"verdict,omitempty"` // set by auto-verify (CLI), if run
}

// Run appends prompt to the transcript and drives the loop until the model
// completes, a breaker trips, or a cap is hit. Every exit path is a structured
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

	gov := governor.New(a.govLimits) // fresh per task
	a.messages = append(a.messages, provider.Message{Role: "user", Text: prompt})

	for {
		turn, halt := gov.BeginTurn()
		if halt != nil {
			return a.halted(*halt), nil
		}

		// Context Ledger: bound the transcript, never silently. Runs before the
		// request is built so the smaller transcript is what gets sent.
		if a.hist != nil {
			if compacted, did, info := a.hist.MaybeCompact(a.messages); did {
				a.messages = compacted
				a.led.Append(ledger.Entry{Turn: turn, Event: ledger.EventInfo, Detail: "context compacted: " + info})
				a.emit(Event{Kind: "context", Turn: turn, Detail: info})
			}
		}

		// Gate 1 (preflight): conservative estimate before the model fires.
		if err := a.bud.Preflight(a.cfg.Model, a.cfg.EstInputTokens, a.cfg.EstOutputTokens); err != nil {
			return a.budgetHalt(err, turn), nil
		}

		req := provider.Request{System: a.cfg.System, Model: a.cfg.Model, Messages: a.messages, Tools: DefaultToolSpecs()}
		// Bound this request's OUTPUT by the remaining token budget. Input
		// overshoot (a large transcript) is still caught post-turn by Record,
		// which halts before the next turn fires.
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
		if resp.Text != "" {
			a.emit(Event{Kind: "text", Turn: turn, Text: resp.Text})
		}
		if capErr != nil {
			return a.budgetHalt(capErr, turn), nil
		}

		// Record the assistant turn in the transcript.
		a.messages = append(a.messages, provider.Message{Role: "assistant", Text: resp.Text, ToolCalls: resp.ToolCalls})

		// No tool calls => the model produced its final answer.
		if len(resp.ToolCalls) == 0 {
			a.logf(turn, ledger.EventInfo, "completed")
			return Outcome{Stop: StopCompleted, Reason: resp.Text, Turns: turn, Usage: a.bud.Usage()}, nil
		}

		madeProgress := false
		results := make([]provider.ToolResult, 0, len(resp.ToolCalls))
		var pendingHalt *governor.HaltReason
		for i, call := range resp.ToolCalls {
			if h := gov.RecordToolCall(call.Signature); h != nil {
				// Halt before running this call. Emit error results for it and
				// every remaining call so no tool_use is left without a matching
				// tool_result — otherwise the persisted transcript becomes
				// invalid and every later turn in the session would be rejected.
				for _, c := range resp.ToolCalls[i:] {
					results = append(results, provider.ToolResult{ID: c.ID, Content: "skipped: governor " + h.Code, IsError: true})
				}
				pendingHalt = h
				break
			}
			a.emit(Event{Kind: "tool_call", Turn: turn, Tool: call.Name, Detail: argSummary(call.Args)})
			res := a.exec.Execute(ctx, call.Name, call.Args)
			a.led.Append(ledger.Entry{
				Turn: turn, Event: ledger.EventTool,
				Detail: fmt.Sprintf("%s success=%t", call.Name, res.Success),
			})
			a.emit(Event{Kind: "tool_result", Turn: turn, Tool: call.Name, OK: res.Success, Detail: truncate(res.Output, 100)})
			// Any successful tool call (not just an edit) counts as progress, so
			// legitimately read-only/exploratory work is not falsely halted.
			if res.Success {
				madeProgress = true
			}
			results = append(results, provider.ToolResult{ID: call.ID, Content: res.Output, IsError: !res.Success})
			if res.IsEdit {
				if h := gov.RecordEdit(res.Success); h != nil {
					for _, c := range resp.ToolCalls[i+1:] {
						results = append(results, provider.ToolResult{ID: c.ID, Content: "skipped: governor " + h.Code, IsError: true})
					}
					pendingHalt = h
					break
				}
			}
		}

		// Always feed back a COMPLETE set of tool results (one per tool_use),
		// even on a halt, so the transcript stays valid for the next turn.
		a.messages = append(a.messages, provider.Message{Role: "user", ToolResults: results})

		if pendingHalt != nil {
			return a.halted(*pendingHalt), nil
		}
		if h := gov.RecordTurnProgress(madeProgress); h != nil {
			return a.halted(*h), nil
		}
	}
}

func (a *Agent) emit(e Event) {
	if a.obs != nil {
		a.obs(e)
	}
}

func (a *Agent) halted(h governor.HaltReason) Outcome {
	a.led.Append(ledger.Entry{Turn: h.Turn, Event: ledger.EventHalt, Detail: h.Code + ": " + h.Detail})
	a.emit(Event{Kind: "halt", Turn: h.Turn, Detail: h.Code + ": " + h.Detail})
	return Outcome{Stop: h.Code, Reason: h.Detail, Turns: h.Turn, Usage: a.bud.Usage()}
}

func (a *Agent) budgetHalt(err error, turn int) Outcome {
	a.led.Append(ledger.Entry{Turn: turn, Event: ledger.EventHalt, Detail: "budget: " + err.Error()})
	a.emit(Event{Kind: "budget", Turn: turn, Detail: err.Error()})
	return Outcome{Stop: StopBudget, Reason: err.Error(), Turns: turn, Usage: a.bud.Usage()}
}

func (a *Agent) logf(turn int, event, detail string) {
	a.led.Append(ledger.Entry{Turn: turn, Event: event, Detail: detail})
}

// argSummary renders tool args compactly for the activity feed (no big blobs).
func argSummary(args map[string]string) string {
	var parts []string
	for _, k := range []string{"file", "command", "old_string"} {
		if v, ok := args[k]; ok {
			parts = append(parts, k+"="+truncate(v, 48))
		}
	}
	return strings.Join(parts, " ")
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// DefaultToolSpecs are the tools advertised to the model. The executor still
// gates each behind permissions.
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
			Name:        "edit_file",
			Description: "Replace an exact snippet in a file. Prefer this over write_file for edits. old_string must match a unique block; whitespace-only differences are tolerated.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":        strProp("path to the file to edit"),
					"old_string":  strProp("the exact existing text to replace (must be unique unless replace_all)"),
					"new_string":  strProp("the replacement text"),
					"replace_all": map[string]any{"type": "boolean", "description": "replace every occurrence (default false)"},
				},
				"required": []string{"file", "old_string", "new_string"},
			},
		},
		{
			Name:        "write_file",
			Description: "Write (overwrite) a whole file. Use for new files; prefer edit_file for changes.",
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
