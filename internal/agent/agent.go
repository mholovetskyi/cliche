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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mholovetskyi/cliche/internal/budget"
	"github.com/mholovetskyi/cliche/internal/governor"
	"github.com/mholovetskyi/cliche/internal/history"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/pricing"
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/shell"
	"github.com/mholovetskyi/cliche/internal/tools"
)

// Config holds per-run agent settings.
type Config struct {
	System string
	// SystemSuffix, if set, is appended to System at the START OF EACH TURN, so a
	// live-mutable note (e.g. the active personality) takes effect on the next
	// message without rebuilding the agent. Keep it small and stable — it only
	// changes the cached system block when its value actually changes.
	SystemSuffix func() string
	Model        string
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
	// MaxSubagentDepth caps subagent nesting (0 disables subagents). A depth-d
	// agent may spawn children up to depth MaxSubagentDepth.
	MaxSubagentDepth int
	// SubagentModel, when set, routes delegated subagents to a different (e.g.
	// cheaper) model on the same provider — the strong model plans and delegates
	// grunt work down. Empty means subagents use the same model as the parent.
	SubagentModel string
}

// Agent ties the Trust Kernel around a provider and tool executor.
type Agent struct {
	prov          provider.Provider
	bud           *budget.Kernel
	govLimits     governor.Limits
	led           *ledger.Ledger
	exec          tools.Executor
	cfg           Config
	obs           Observer
	hist          *history.Manager
	messages      []provider.Message // persists across Run calls (the session transcript)
	ledgerWarned  bool               // emit at most one warning if audit writes fail
	depth         int                // subagent nesting depth (0 = top-level)
	emitMu        *sync.Mutex        // serializes observer output across parallel subagents
	mcp           MCP                // optional external MCP tools (nil if none)
	pendingImages []provider.Image   // images attached to the NEXT user turn, then cleared
}

// AttachImages queues images to ride on the next Run's user message (for
// vision-capable models). Consumed and cleared by that Run.
func (a *Agent) AttachImages(imgs []provider.Image) { a.pendingImages = imgs }

// New builds an Agent. EstInputTokens/EstOutputTokens default to conservative
// values if unset. A fresh Governor is created per Run from govLimits.
func New(p provider.Provider, b *budget.Kernel, govLimits governor.Limits, l *ledger.Ledger, e tools.Executor, cfg Config) *Agent {
	if cfg.EstInputTokens <= 0 {
		cfg.EstInputTokens = 5000
	}
	if cfg.EstOutputTokens <= 0 {
		cfg.EstOutputTokens = 1000
	}
	a := &Agent{prov: p, bud: b, govLimits: govLimits, led: l, exec: e, cfg: cfg, emitMu: &sync.Mutex{}}
	if cfg.ContextLimitTokens > 0 {
		a.hist = history.New(cfg.ContextLimitTokens, cfg.ContextKeepRecent)
	}
	return a
}

// MCP is an optional source of external Model Context Protocol tools, already
// namespaced ("mcp__<server>__<tool>") and permission-gated by the caller.
type MCP interface {
	Tools() []provider.ToolSpec
	Call(ctx context.Context, name string, raw json.RawMessage) tools.Result
}

// SetObserver registers a streaming observer for live activity.
func (a *Agent) SetObserver(obs Observer) { a.obs = obs }

// SetModel changes the model used for subsequent turns (e.g. an in-session
// /model switch). The budget/governor are unaffected.
func (a *Agent) SetModel(model string) { a.cfg.Model = model }

// SetProvider swaps the model backend mid-session (used by /provider to switch
// providers without restarting). The transcript, budget, and governor are
// untouched — only where completions are sent changes.
func (a *Agent) SetProvider(p provider.Provider, model string) {
	a.prov = p
	a.cfg.Model = model
}

// Model returns the model currently in use.
func (a *Agent) Model() string { return a.cfg.Model }

// SetMCP attaches an MCP tool source (its tools are advertised and routed).
func (a *Agent) SetMCP(m MCP) { a.mcp = m }

// MCP returns the agent's current MCP source (nil if none), so the CLI can
// hot-attach a newly connected server to the live session.
func (a *Agent) MCP() MCP { return a.mcp }

// Usage returns the session-cumulative budget usage.
func (a *Agent) Usage() budget.Usage { return a.bud.Usage() }

// Transcript returns the current conversation messages (for session persistence).
func (a *Agent) Transcript() []provider.Message { return a.messages }

// Restore loads a persisted transcript (for `chat --resume`). The budget usage
// is seeded separately via the kernel so the session-wide cap stays honest
// across resumes.
func (a *Agent) Restore(msgs []provider.Message, usage budget.Usage) {
	a.messages = msgs
	a.bud.Preload(usage)
}

// RestoreTranscript swaps in a saved transcript WITHOUT touching the budget —
// for switching sessions mid-process (e.g. /resume in chat). The live spend cap
// must keep counting this process's cumulative usage and can never be reset
// downward by loading a cheaper session, or the hard cap would be defeatable.
func (a *Agent) RestoreTranscript(msgs []provider.Message) {
	a.messages = msgs
}

// Limits returns the budget limits.
func (a *Agent) Limits() budget.Limits { return a.bud.Limits() }

// SetLimits updates the budget caps live (the server changes these only between
// runs). The running spend tally is preserved, so a raised cap grants headroom
// immediately and a lowered one applies on the next turn.
func (a *Agent) SetLimits(l budget.Limits) { a.bud.SetLimits(l) }

// GovernorLimits returns the per-run governor caps (turns, wall-clock, etc.).
func (a *Agent) GovernorLimits() governor.Limits { return a.govLimits }

// SetGovernorLimits updates the governor caps; a fresh governor is built per Run,
// so the change takes effect on the next turn.
func (a *Agent) SetGovernorLimits(g governor.Limits) { a.govLimits = g }

// Reset clears the conversation transcript and any recoverable compaction
// snapshot (the budget is preserved).
func (a *Agent) Reset() {
	a.messages = nil
	if a.hist != nil {
		a.hist.Reset()
	}
}

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
	StopCancelled = "cancelled"
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
func (a *Agent) Run(ctx context.Context, prompt string) (out Outcome, err error) {
	// A panic anywhere in the turn — a buggy tool, provider, or MCP server — must
	// NOT take down a long-running `cliche serve` / cron / telegram process. Recover
	// it into a structured error so the loop fails the run, not the whole daemon.
	defer func() {
		if r := recover(); r != nil {
			a.emit(Event{Kind: "halt", Detail: fmt.Sprintf("recovered from an internal error: %v", r)})
			out = Outcome{Stop: StopError, Reason: fmt.Sprintf("internal error (recovered): %v", r), Usage: a.bud.Usage()}
			err = fmt.Errorf("agent panic recovered: %v", r)
		}
	}()
	// Bound the whole run (and any single tool command) by the wall-clock
	// limit, so a hung command cannot outlast the Governor's breaker, which is
	// only checked between turns.
	if a.cfg.MaxWallClock > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.cfg.MaxWallClock)
		defer cancel()
	}

	gov := governor.New(a.govLimits) // fresh per task
	a.messages = append(a.messages, provider.Message{Role: "user", Text: prompt, Images: a.pendingImages})
	a.pendingImages = nil // images ride exactly one turn

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
				a.rec(ledger.Entry{Turn: turn, Event: ledger.EventInfo, Detail: "context compacted: " + info})
				a.emit(Event{Kind: "context", Turn: turn, Detail: info})
			}
		}

		// Gate 1 (preflight): conservative estimate before the model fires.
		if err := a.bud.Preflight(a.cfg.Model, a.cfg.EstInputTokens, a.cfg.EstOutputTokens); err != nil {
			return a.budgetHalt(err, turn), nil
		}

		sys := a.cfg.System
		if a.cfg.SystemSuffix != nil {
			sys += a.cfg.SystemSuffix() // live note (e.g. the active personality), re-read each turn
		}
		req := provider.Request{System: sys, Model: a.cfg.Model, Messages: a.messages, Tools: a.toolSpecs()}
		// Bound this request's OUTPUT by the remaining token budget (the tightest
		// across the kernel chain, so a scoped subagent is bounded too). Input
		// overshoot is still caught post-turn by Record. Remaining() returns 0
		// when nothing in the chain has a token cap.
		if rem, _ := a.bud.Remaining(); rem > 0 {
			req.MaxOutputTokens = rem
		}

		// Stream text deltas live at the top level only. Subagents (depth>0)
		// share the observer; streaming their tokens would interleave and garble
		// the parent's output, so they stay request/response.
		streamed := false
		if a.obs != nil && a.depth == 0 {
			req.OnDelta = func(t string) {
				streamed = true
				a.emit(Event{Kind: "delta", Turn: turn, Text: t})
			}
		}

		resp, err := a.prov.Complete(ctx, req)
		if err != nil {
			// Roll back a dangling fresh user prompt so a later task in the same
			// session can't produce two consecutive user messages.
			if n := len(a.messages); n > 0 && a.messages[n-1].Role == "user" && a.messages[n-1].Text != "" && len(a.messages[n-1].ToolResults) == 0 {
				a.messages = a.messages[:n-1]
			}
			if ctx.Err() != nil {
				// A wall-clock deadline mid-turn is a structured governor halt;
				// an external cancellation (SIGINT) is a structured "cancelled".
				if a.cfg.MaxWallClock > 0 && errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return a.halted(governor.HaltReason{Code: "max_wallclock", Detail: "wall-clock limit exceeded mid-turn", Turn: turn}), nil
				}
				a.rec(ledger.Entry{Turn: turn, Event: ledger.EventHalt, Detail: "cancelled"})
				a.emit(Event{Kind: "halt", Turn: turn, Detail: "cancelled"})
				return Outcome{Stop: StopCancelled, Reason: "interrupted", Turns: turn, Usage: a.bud.Usage()}, nil
			}
			a.rec(ledger.Entry{Turn: turn, Event: ledger.EventInfo, Detail: "provider error: " + err.Error()})
			return Outcome{Stop: StopError, Reason: err.Error(), Turns: turn, Usage: a.bud.Usage()}, err
		}

		// Gate 2 (post-turn): record ACTUAL usage. Catches a fat-completion
		// turn that blew the pre-flight estimate; halts before the next turn.
		// Prompt-cache reads/writes count toward the hard token cap but bill at
		// the discounted rate, so the dollar estimate reflects what caching saves.
		u := resp.Usage
		capErr := a.bud.RecordCached(a.cfg.Model, u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens)
		price, _ := pricing.Lookup(a.cfg.Model)
		a.rec(ledger.Entry{
			Turn: turn, Event: ledger.EventTurn, Model: a.cfg.Model,
			InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
			USD:    price.CostUSDCached(u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens),
			Detail: truncate(resp.Text, 120),
		})
		if u.CacheReadTokens > 0 {
			a.emit(Event{Kind: "cache", Turn: turn, Detail: fmt.Sprintf("%d tokens served from cache (billed ~0.1×)", u.CacheReadTokens)})
		}
		// Emit the full text only when it wasn't already streamed live (deltas).
		if resp.Text != "" && !streamed {
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
		var shotImages []provider.Image // screenshots a tool returned this turn, fed to the model as vision
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
			var res tools.Result
			switch call.Name {
			case "spawn_subagent":
				res = a.spawnSubagent(ctx, call.Args)
			case "spawn_subagents":
				res = a.spawnSubagents(ctx, call.Args)
			case "update_plan":
				res = a.updatePlan(turn, call.Args)
			default:
				if a.mcp != nil && strings.HasPrefix(call.Name, "mcp__") {
					res = a.mcp.Call(ctx, call.Name, call.Raw)
				} else {
					res = a.exec.Execute(ctx, call.Name, call.Args)
				}
			}
			// Trust-Kernel input boundary: bound the size and redact secrets from
			// EVERY tool result — built-in, MCP, or subagent — before it reaches the
			// model or the ledger. Untrusted output can't flood the context or leak a
			// credential in plain sight.
			res.Output = tools.SanitizeOutput(res.Output)
			// Record an attributable target (path or truncated command) — but
			// never file contents or old_string (which could carry secrets).
			target := call.Args["file"]
			if target == "" {
				target = truncate(call.Args["command"], 80)
			}
			if target == "" {
				target = truncate(call.Args["url"], 80)
			}
			a.rec(ledger.Entry{
				Turn: turn, Event: ledger.EventTool,
				Detail: fmt.Sprintf("%s success=%t %s", call.Name, res.Success, target),
			})
			resDetail := resultSummary(res.Output)
			if !res.Success {
				resDetail = truncate(res.Output, 200) // failures: show more of the error
			}
			// Surface any images a tool returned (e.g. screenshots) to the live UI
			// feed as data: URLs — and feed the same bytes to the model as vision.
			var shotURLs []string
			for _, im := range res.Images {
				shotImages = append(shotImages, provider.Image{MediaType: im.MediaType, Data: im.Data})
				shotURLs = append(shotURLs, "data:"+im.MediaType+";base64,"+base64.StdEncoding.EncodeToString(im.Data))
			}
			a.emit(Event{Kind: "tool_result", Turn: turn, Tool: call.Name, OK: res.Success, IsEdit: res.IsEdit, Path: call.Args["file"], Detail: resDetail, Images: shotURLs})
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
		a.messages = append(a.messages, provider.Message{Role: "user", ToolResults: results, Images: shotImages})

		if pendingHalt != nil {
			return a.halted(*pendingHalt), nil
		}
		if h := gov.RecordTurnProgress(madeProgress); h != nil {
			return a.halted(*h), nil
		}
	}
}

func (a *Agent) emit(e Event) {
	if a.obs == nil {
		return
	}
	a.emitMu.Lock()
	defer a.emitMu.Unlock()
	a.obs(e)
}

// rec appends to the audit ledger, surfacing (once) a warning if the write
// fails so a broken audit trail is never silently ignored.
func (a *Agent) rec(e ledger.Entry) {
	if err := a.led.Append(e); err != nil && !a.ledgerWarned {
		a.ledgerWarned = true
		a.emit(Event{Kind: "warn", Detail: "audit ledger write failed: " + err.Error()})
	}
}

func (a *Agent) halted(h governor.HaltReason) Outcome {
	a.rec(ledger.Entry{Turn: h.Turn, Event: ledger.EventHalt, Detail: h.Code + ": " + h.Detail})
	a.emit(Event{Kind: "halt", Turn: h.Turn, Detail: h.Code + ": " + h.Detail})
	return Outcome{Stop: h.Code, Reason: h.Detail, Turns: h.Turn, Usage: a.bud.Usage()}
}

func (a *Agent) budgetHalt(err error, turn int) Outcome {
	a.rec(ledger.Entry{Turn: turn, Event: ledger.EventHalt, Detail: "budget: " + err.Error()})
	a.emit(Event{Kind: "budget", Turn: turn, Detail: err.Error()})
	return Outcome{Stop: StopBudget, Reason: err.Error(), Turns: turn, Usage: a.bud.Usage()}
}

func (a *Agent) logf(turn int, event, detail string) {
	a.rec(ledger.Entry{Turn: turn, Event: event, Detail: detail})
}

// argSummary renders the primary target of a tool call for the activity feed:
// the file, command, URL, search pattern, path, or subagent prompt — no key=
// prefixes and no big blobs (the verb column already names the action; a diff is
// shown at approval time). The key order is the salience order, and the curated
// list covers every built-in tool so the feed and spinner always name a target
// rather than showing a bare verb.
func argSummary(args map[string]string) string {
	for _, k := range []string{"file", "command", "url", "pattern", "path", "prompt", "query"} {
		if v := strings.TrimSpace(args[k]); v != "" {
			return truncate(v, 64)
		}
	}
	return ""
}

// truncate clips to n runes (never mid-rune, so the live feed can't emit a
// replacement char), appending an ellipsis when it cuts.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "…"
}

// resultSummary turns a tool's raw output into a quiet, quantified confirmation
// for the feed: a short single-line result is shown as-is (e.g. "wrote foo.go"),
// multi-line output is reduced to a line count, empty output reads "ok".
func resultSummary(out string) string {
	out = strings.TrimSpace(out)
	switch {
	case out == "":
		return "ok"
	case !strings.Contains(out, "\n"):
		return truncate(out, 72)
	default:
		return fmt.Sprintf("%d lines", strings.Count(out, "\n")+1)
	}
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
			Description: "Read a UTF-8 text file. Returns the whole file, or a line range via offset/limit. Large files are truncated to the first 2000 lines unless you pass a limit — page through the rest with offset.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":   strProp("path to the file to read"),
					"offset": map[string]any{"type": "integer", "description": "1-based line to start reading from (optional)"},
					"limit":  map[string]any{"type": "integer", "description": "maximum number of lines to read (optional)"},
				},
				"required": []string{"file"},
			},
		},
		{
			Name:        "search_files",
			Description: "Search file contents for a regular expression (like grep), returning matching lines as 'path:line: text'. Confined to the project; skips .git/node_modules and binary files. Prefer this over running grep via run_command.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern":     strProp("RE2 regular expression to search for"),
					"path":        strProp("optional file or directory to search under (default: project root)"),
					"glob":        strProp("optional glob to limit which files are searched, e.g. '*.go' or 'internal/**/*.go'"),
					"ignore_case": map[string]any{"type": "boolean", "description": "case-insensitive match (default false)"},
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name:        "find_files",
			Description: "Find files by glob pattern, returning matching paths. '**' spans directories; a pattern without a slash (e.g. '*.go') matches by base name at any depth. Confined to the project; skips .git/node_modules. Prefer this over running find via run_command.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": strProp("glob pattern, e.g. '*.go', '**/*_test.go', 'internal/**/*.json'"),
					"path":    strProp("optional directory to search under (default: project root)"),
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name:        "list_files",
			Description: "List the immediate entries of a directory (non-recursive); directories end with '/'. Confined to the project root.",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": strProp("directory to list (default: project root)")},
			},
		},
		{
			Name:        "diagnostics",
			Description: "Run the project's REAL type-checker / compiler / linter and return findings as structured file:line:col errors — Go (go build + vet), Node/TS (tsc or the package.json typecheck/lint script), Python (mypy/ruff), Rust (cargo check). Auto-detects the toolchain and only runs checkers that are installed. Use this to VERIFY a change compiles and type-checks after editing — prefer it over running go build / tsc yourself via run_command. Read-only.",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": strProp("optional subproject directory to check (default: project root)")},
			},
		},
		{
			Name:        "find_symbol",
			Description: "Find where a symbol is DEFINED (op=definition, default) or REFERENCED (op=references) across the codebase — go-to-definition / find-references without a language server. Go uses precise AST parsing; other languages use declaration patterns. Use it to navigate a real codebase instead of grepping blindly.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":     strProp("the symbol name to find (e.g. a function, type, or variable)"),
					"op":       strProp("'definition' (default) or 'references'"),
					"language": strProp("optional language filter: go | ts | js | py | rust | java"),
					"path":     strProp("optional directory to search under (default: project root)"),
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "web_fetch",
			Description: "Fetch a URL and return its text (HTML is reduced to readable text). Use to pull current docs/specs into context. Network access is permission-gated.",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"url": strProp("the http(s) URL to fetch")},
				"required":   []string{"url"},
			},
		},
		{
			Name:        "clone_site",
			Description: "Clone an existing website. Give it an http(s) URL and it returns BOTH a screenshot of the rendered page AND its text content, so you can recreate the site as a clean, modern, responsive app. Use it whenever the user asks to clone, copy, recreate, redesign, or 'build me something like' a website and names a URL. After you rebuild it, screenshot YOUR result and compare it to the original, iterating until it matches and looks world-class. Network egress — gated by the egress allowlist and approval, just like web_fetch.",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"url": strProp("the http(s) URL of the website to clone")},
				"required":   []string{"url"},
			},
		},
		{
			Name:        "scaffold_backend",
			Description: "Give the app a REAL backend — a Supabase (hosted Postgres + auth) integration: a typed client, an env template, a starter schema with row-level security, and a connect guide. Use this whenever the app needs to persist data, authenticate users, or do anything a static frontend can't. One call writes the whole scaffold (existing files are never overwritten); afterwards install @supabase/supabase-js and wire the client into your components. If a Supabase MCP connector is available, it can create the project and apply the schema.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": strProp("backend kind — currently \"supabase\" (the default)"),
					"dir":  strProp("app directory to scaffold into, relative to the project root (default \".\")"),
				},
			},
		},
		{
			Name:        "screenshot",
			Description: "Render a web UI with a headless browser and return a screenshot image so you can SEE the result and judge it. ALWAYS use this after building or changing any visual UI, then critique what you see (layout, spacing, type, color, polish) and iterate until it looks world-class — do not ship a UI you have not looked at. The target is a project-relative file (default index.html) or a localhost URL. Returns the image directly to you.",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target": strProp("project-relative file to render (e.g. index.html or dist/index.html), or a http://localhost URL; defaults to index.html"),
					"width":  strProp("optional viewport width in px (default 1366)"),
					"height": strProp("optional viewport height in px (default 900)"),
				},
			},
		},
		{
			Name:        "edit_file",
			Description: "Replace an exact snippet in a file. Prefer this over write_file for edits. Read the file first and copy old_string verbatim (it must match a unique block; whitespace-only differences are tolerated). Keep new_string syntactically complete — a result that doesn't parse is rejected and the file is left unchanged.",
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
			Description: "Write (overwrite) a whole file, creating any parent directories as needed (so you can scaffold new folders). Use for new files; prefer edit_file for changes.",
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
			Description: "Run a shell command in the project directory and return its output. Shell: " + shell.Describe() + ".",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"command": strProp("the shell command to run")},
				"required":   []string{"command"},
			},
		},
		{
			Name:        "remember",
			Description: "Propose a durable fact about THIS project for cross-session memory (a convention, decision, gotcha, or build/test command) so future sessions start knowing it. The user approves the save (it's gated like a write), so phrase it as a clean, self-contained fact and don't be surprised if a proposal is declined. Use sparingly for things genuinely worth persisting. Stored in a human-readable .cliche/memory.md the user can edit.",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"fact": strProp("the durable fact to remember, as one concise sentence")},
				"required":   []string{"fact"},
			},
		},
		{
			Name:        "remember_user",
			Description: "Propose a durable fact about the USER — a lasting preference or how they like to work (their stack, conventions they always want, communication style) — for a cross-PROJECT profile loaded in EVERY future session, in any repo. The user approves the save (gated like a write). Use for lasting things about the person. Project-specific facts go to remember instead; don't use for transient one-off requests.",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"fact": strProp("the durable user preference, as one concise sentence")},
				"required":   []string{"fact"},
			},
		},
		{
			Name:        "recall",
			Description: "Search THIS project's PAST sessions for relevant context — earlier decisions, discussions, or work. Use when the user refers to earlier work ('like we did before', 'what did we decide about X') or you need history beyond the current conversation. Returns matching snippets with dates.",
			Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": strProp("what to look for across past sessions")},
				"required":   []string{"query"},
			},
		},
		{
			Name:        "save_skill",
			Description: "After you complete a NON-TRIVIAL, REUSABLE procedure (a multi-step workflow you'd repeat — e.g. how to scaffold this kind of project, run this repo's release, or debug a recurring issue), distill it into a reusable skill for future sessions. Provide a short kebab-case name, a one-line description of WHEN to use it, and step-by-step instructions as the content. Saved to .cliche/skills/<name>/SKILL.md and loaded automatically next time. The user must approve it (it's a write), so it's auditable. Do NOT use for one-off tasks, trivial actions, or facts (use remember for facts).",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":        strProp("short kebab-case skill name, e.g. scaffold-vite-app"),
					"description": strProp("one line: when this skill should be used"),
					"content":     strProp("the reusable instructions, as clear step-by-step Markdown"),
				},
				"required": []string{"name", "description", "content"},
			},
		},
	}
}

// toolSpecs is the tool set advertised for this agent — DefaultToolSpecs plus
// spawn_subagent when nesting is still permitted at this depth.
func (a *Agent) toolSpecs() []provider.ToolSpec {
	specs := DefaultToolSpecs()
	specs = append(specs, updatePlanSpec())
	if a.depth < a.cfg.MaxSubagentDepth {
		specs = append(specs, spawnSubagentSpec(), spawnSubagentsSpec())
	}
	if a.mcp != nil {
		specs = append(specs, a.mcp.Tools()...)
	}
	return specs
}

// updatePlanSpec advertises the live progress checklist.
func updatePlanSpec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "update_plan",
		Description: "Maintain a short, user-visible progress checklist for a multi-step task. Call it ONCE up front to lay out the 3–8 concrete steps, then call it again to flip a step's status as you go — exactly one step \"doing\" at a time, finished steps \"done\". The user watches this tick off in real time, so keep titles short and imperative. Use it for any task that takes more than a couple of tool calls (building an app, a multi-file change, a clone). Don't use it for trivial one-shot answers.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"steps": map[string]any{
					"type":        "array",
					"description": "the full checklist, in order — resend the whole list each call with updated statuses",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"title":  map[string]any{"type": "string", "description": "short imperative step, e.g. 'Build the hero section'"},
							"status": map[string]any{"type": "string", "enum": []string{"pending", "doing", "done"}, "description": "pending | doing | done"},
						},
						"required": []string{"title", "status"},
					},
				},
			},
			"required": []string{"steps"},
		},
	}
}

// updatePlan records the agent's checklist and streams it to the UI so the user
// can watch progress. It's pure signal — no filesystem or budget effect.
func (a *Agent) updatePlan(turn int, args map[string]string) tools.Result {
	var steps []PlanStep
	if err := json.Unmarshal([]byte(args["steps"]), &steps); err != nil || len(steps) == 0 {
		return tools.Result{Output: "update_plan: steps must be a non-empty JSON array of {title,status}", Success: false}
	}
	for i := range steps {
		steps[i].Title = strings.TrimSpace(steps[i].Title)
		switch steps[i].Status {
		case "doing", "done", "pending":
		default:
			steps[i].Status = "pending"
		}
	}
	a.emit(Event{Kind: "plan", Turn: turn, Plan: steps})
	done := 0
	for _, s := range steps {
		if s.Status == "done" {
			done++
		}
	}
	return tools.Result{Output: fmt.Sprintf("plan updated: %d/%d steps done", done, len(steps)), Success: true}
}

func spawnSubagentSpec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "spawn_subagent",
		Description: "Delegate a self-contained subtask to a subagent that has its OWN fresh context and a scoped budget (drawn from, and bounded by, the session budget). Use for isolated work (investigate a file, draft a function). Returns the subagent's final summary.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt":     map[string]any{"type": "string", "description": "the subtask for the subagent"},
				"max_usd":    map[string]any{"type": "number", "description": "optional dollar cap for this subagent"},
				"max_tokens": map[string]any{"type": "integer", "description": "optional token cap for this subagent"},
			},
			"required": []string{"prompt"},
		},
	}
}

func spawnSubagentsSpec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "spawn_subagents",
		Description: "Run SEVERAL subagents in PARALLEL, each on its own isolated subtask with a scoped budget. Use for independent work that can proceed concurrently (e.g. investigate three files at once). Returns all summaries together.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tasks": map[string]any{
					"type":        "array",
					"description": "the subtasks to run concurrently",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"prompt":     map[string]any{"type": "string", "description": "the subtask"},
							"max_usd":    map[string]any{"type": "number", "description": "optional dollar cap"},
							"max_tokens": map[string]any{"type": "integer", "description": "optional token cap"},
						},
						"required": []string{"prompt"},
					},
				},
			},
			"required": []string{"tasks"},
		},
	}
}

// spawnSubagent runs a child agent on an isolated subtask with a scoped budget
// and returns its summary as a tool result.
func (a *Agent) spawnSubagent(ctx context.Context, args map[string]string) tools.Result {
	if a.depth >= a.cfg.MaxSubagentDepth {
		return tools.Result{Output: "subagents are not available at this depth", Success: false}
	}
	prompt := strings.TrimSpace(args["prompt"])
	if prompt == "" {
		return tools.Result{Output: "spawn error: empty subagent prompt", Success: false}
	}
	var sub budget.Limits
	if v, err := strconv.Atoi(strings.TrimSpace(args["max_tokens"])); err == nil && v > 0 {
		sub.MaxTokens = v
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(args["max_usd"]), 64); err == nil && v > 0 {
		sub.MaxUSD = v
	}

	child := a.newChild(sub)
	o, err := child.Run(ctx, prompt)
	if err != nil {
		return tools.Result{Output: "subagent error: " + err.Error(), Success: false}
	}
	out := fmt.Sprintf("subagent finished (%s, %d turns, ~$%.4f): %s",
		o.Stop, o.Turns, o.Usage.USD, truncate(o.Reason, 2000))
	return tools.Result{Output: out, Success: o.Stop == StopCompleted}
}

// newChild builds a subagent that shares the provider, ledger, and tool
// executor (so confinement and permissions are inherited), gets a budget scoped
// under the parent's, and starts with a FRESH, isolated context.
func (a *Agent) newChild(sub budget.Limits) *Agent {
	c := New(a.prov, a.bud.Scoped(sub), a.govLimits, a.led, a.exec, a.cfg)
	c.depth = a.depth + 1
	c.obs = a.obs
	c.emitMu = a.emitMu // share so concurrent siblings serialize their output
	c.mcp = a.mcp       // subagents inherit the same MCP tools
	// Model routing: delegated work runs on the configured subagent model (same
	// provider). The override is inherited, so deeper descendants stay on it too.
	if a.cfg.SubagentModel != "" {
		c.cfg.Model = a.cfg.SubagentModel
	}
	return c
}

type subTask struct {
	Prompt    string  `json:"prompt"`
	MaxUSD    float64 `json:"max_usd"`
	MaxTokens int     `json:"max_tokens"`
}

// maxParallelSubagents bounds fan-out per call (budget/governor still bound spend).
const maxParallelSubagents = 8

// spawnSubagents runs several subagents CONCURRENTLY, each isolated with its own
// scoped budget, and returns their combined summaries. Spend from all of them
// bubbles into (and is bounded by) the shared session budget.
func (a *Agent) spawnSubagents(ctx context.Context, args map[string]string) tools.Result {
	if a.depth >= a.cfg.MaxSubagentDepth {
		return tools.Result{Output: "subagents are not available at this depth", Success: false}
	}
	var tasks []subTask
	if err := json.Unmarshal([]byte(args["tasks"]), &tasks); err != nil {
		return tools.Result{Output: "spawn error: 'tasks' must be a JSON array of {prompt,...}: " + err.Error(), Success: false}
	}
	if len(tasks) == 0 {
		return tools.Result{Output: "spawn error: no tasks provided", Success: false}
	}
	truncated := false
	if len(tasks) > maxParallelSubagents {
		tasks = tasks[:maxParallelSubagents]
		truncated = true
	}

	results := make([]string, len(tasks))
	var wg sync.WaitGroup
	for i, tk := range tasks {
		if strings.TrimSpace(tk.Prompt) == "" {
			results[i] = fmt.Sprintf("task %d skipped: empty prompt", i+1)
			continue
		}
		wg.Add(1)
		go func(i int, tk subTask) {
			defer wg.Done()
			child := a.newChild(budget.Limits{MaxTokens: tk.MaxTokens, MaxUSD: tk.MaxUSD})
			o, err := child.Run(ctx, tk.Prompt)
			if err != nil {
				results[i] = fmt.Sprintf("task %d error: %v", i+1, err)
				return
			}
			results[i] = fmt.Sprintf("task %d (%s, ~$%.4f): %s", i+1, o.Stop, o.Usage.USD, truncate(o.Reason, 800))
		}(i, tk)
	}
	wg.Wait()

	out := strings.Join(results, "\n")
	if truncated {
		out += fmt.Sprintf("\n(note: only the first %d tasks were run)", maxParallelSubagents)
	}
	return tools.Result{Output: out, Success: true}
}
