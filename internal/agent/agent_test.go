package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mholovetskyi/cliche/internal/budget"
	"github.com/mholovetskyi/cliche/internal/governor"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/tools"
)

type errProvider struct{}

func (errProvider) Name() string  { return "err" }
func (errProvider) Model() string { return "mock" }
func (errProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	return provider.Response{}, errors.New("boom")
}

func firstUserText(msgs []provider.Message) string {
	for _, m := range msgs {
		if m.Role == "user" && m.Text != "" {
			return m.Text
		}
	}
	return ""
}

func hasToolResult(msgs []provider.Message) bool {
	for _, m := range msgs {
		if len(m.ToolResults) > 0 {
			return true
		}
	}
	return false
}

// routingMock branches on the initial prompt: the "parent" run delegates once
// then finishes; any other (child) run finishes immediately.
type routingMock struct{}

func (routingMock) Name() string  { return "routing" }
func (routingMock) Model() string { return "mock" }
func (routingMock) Complete(_ context.Context, req provider.Request) (provider.Response, error) {
	if firstUserText(req.Messages) == "parent" {
		if hasToolResult(req.Messages) {
			return provider.Response{Text: "parent finished", Done: true, Usage: provider.Usage{InputTokens: 100, OutputTokens: 20}}, nil
		}
		return provider.Response{
			Text:      "delegating",
			ToolCalls: []provider.ToolCall{{ID: "s1", Name: "spawn_subagent", Args: map[string]string{"prompt": "child task"}, Signature: "spawn:child"}},
			Usage:     provider.Usage{InputTokens: 200, OutputTokens: 30},
		}, nil
	}
	return provider.Response{Text: "child finished", Done: true, Usage: provider.Usage{InputTokens: 50, OutputTokens: 10}}, nil
}

func TestSubagentDelegationAndBudgetBubbling(t *testing.T) {
	led, _ := ledger.Open(t.TempDir())
	bud := budget.New(budget.Limits{MaxTokens: 1_000_000, MaxUSD: 100})
	a := New(routingMock{}, bud, governor.DefaultLimits(), led, tools.SimExecutor{},
		Config{Model: "mock", MaxSubagentDepth: 2})
	o, err := a.Run(context.Background(), "parent")
	if err != nil {
		t.Fatal(err)
	}
	if o.Stop != StopCompleted || o.Reason != "parent finished" {
		t.Fatalf("want completed/parent finished, got %s/%q", o.Stop, o.Reason)
	}
	// Root budget must include the child's spend (parent 230+120 + child 60).
	if got := bud.Usage().TotalTokens(); got != 410 {
		t.Fatalf("child spend should bubble into the session budget; got %d, want 410", got)
	}
}

// parallelMock delegates to three concurrent subagents on the parent run.
type parallelMock struct{}

func (parallelMock) Name() string  { return "parallel" }
func (parallelMock) Model() string { return "mock" }
func (parallelMock) Complete(_ context.Context, req provider.Request) (provider.Response, error) {
	if firstUserText(req.Messages) == "parent" {
		if hasToolResult(req.Messages) {
			return provider.Response{Text: "parent finished", Done: true, Usage: provider.Usage{InputTokens: 100, OutputTokens: 20}}, nil
		}
		return provider.Response{
			Text: "fanning out",
			ToolCalls: []provider.ToolCall{{
				ID: "p1", Name: "spawn_subagents", Signature: "spawn_subagents:abc",
				Args: map[string]string{"tasks": `[{"prompt":"a"},{"prompt":"b"},{"prompt":"c"}]`},
			}},
			Usage: provider.Usage{InputTokens: 200, OutputTokens: 30},
		}, nil
	}
	return provider.Response{Text: "child done", Done: true, Usage: provider.Usage{InputTokens: 50, OutputTokens: 10}}, nil
}

func TestParallelSubagents(t *testing.T) {
	led, _ := ledger.Open(t.TempDir())
	bud := budget.New(budget.Limits{MaxTokens: 10_000_000, MaxUSD: 1000})
	a := New(parallelMock{}, bud, governor.DefaultLimits(), led, tools.SimExecutor{},
		Config{Model: "mock", MaxSubagentDepth: 2})
	o, err := a.Run(context.Background(), "parent")
	if err != nil {
		t.Fatal(err)
	}
	if o.Stop != StopCompleted || o.Reason != "parent finished" {
		t.Fatalf("want completed/parent finished, got %s/%q", o.Stop, o.Reason)
	}
	// parent 230+120 + three children * 60 = 530, all bubbled into the session.
	if got := bud.Usage().TotalTokens(); got != 530 {
		t.Fatalf("parallel children spend should bubble to the session; got %d, want 530", got)
	}
}

func TestSubagentDepthLimit(t *testing.T) {
	led, _ := ledger.Open(t.TempDir())
	a := New(provider.NewMock("mock", provider.NormalScript(), false),
		budget.New(budget.Limits{MaxTokens: 1000}), governor.DefaultLimits(), led,
		tools.SimExecutor{}, Config{Model: "mock", MaxSubagentDepth: 0})
	for _, s := range a.toolSpecs() {
		if s.Name == "spawn_subagent" {
			t.Fatal("spawn_subagent must not be advertised at depth >= max")
		}
	}
	if res := a.spawnSubagent(context.Background(), map[string]string{"prompt": "x"}); res.Success {
		t.Fatal("spawnSubagent must refuse when depth limit is 0")
	}
}

type stubMCP struct{ called string }

func (s *stubMCP) Tools() []provider.ToolSpec {
	return []provider.ToolSpec{{Name: "mcp__stub__hello", Description: "say hi", Schema: map[string]any{"type": "object"}}}
}
func (s *stubMCP) Call(_ context.Context, name string, _ json.RawMessage) tools.Result {
	s.called = name
	return tools.Result{Output: "hello from " + name, Success: true}
}

type mcpRoutingMock struct{}

func (mcpRoutingMock) Name() string  { return "mcprouting" }
func (mcpRoutingMock) Model() string { return "mock" }
func (mcpRoutingMock) Complete(_ context.Context, req provider.Request) (provider.Response, error) {
	if hasToolResult(req.Messages) {
		return provider.Response{Text: "done", Done: true, Usage: provider.Usage{InputTokens: 10, OutputTokens: 5}}, nil
	}
	return provider.Response{
		ToolCalls: []provider.ToolCall{{ID: "m1", Name: "mcp__stub__hello", Raw: json.RawMessage(`{"x":1}`), Signature: "mcp:hello"}},
		Usage:     provider.Usage{InputTokens: 20, OutputTokens: 5},
	}, nil
}

func TestMCPToolAdvertisedAndRouted(t *testing.T) {
	led, _ := ledger.Open(t.TempDir())
	stub := &stubMCP{}
	a := New(mcpRoutingMock{}, budget.New(budget.Limits{MaxTokens: 1_000_000}),
		governor.DefaultLimits(), led, tools.SimExecutor{}, Config{Model: "mock"})
	a.SetMCP(stub)

	found := false
	for _, s := range a.toolSpecs() {
		if s.Name == "mcp__stub__hello" {
			found = true
		}
	}
	if !found {
		t.Fatal("MCP tool was not advertised in the tool specs")
	}

	o, err := a.Run(context.Background(), "use the mcp tool")
	if err != nil {
		t.Fatal(err)
	}
	if o.Stop != StopCompleted {
		t.Fatalf("want completed, got %s", o.Stop)
	}
	if stub.called != "mcp__stub__hello" {
		t.Fatalf("MCP call was not routed to the adapter, called=%q", stub.called)
	}
}

func TestProviderErrorRollsBackPrompt(t *testing.T) {
	a := newTestAgent(t, errProvider{}, governor.DefaultLimits(),
		budget.Limits{MaxTokens: 1_000_000}, tools.SimExecutor{})
	o, err := a.Run(context.Background(), "do it")
	if err == nil {
		t.Fatal("expected the provider error to propagate")
	}
	if o.Stop != StopError {
		t.Fatalf("want StopError, got %s", o.Stop)
	}
	if len(a.messages) != 0 {
		t.Fatalf("the dangling user prompt should be rolled back, got %d messages", len(a.messages))
	}
}

func newTestAgent(t *testing.T, prov provider.Provider, govLimits governor.Limits, lim budget.Limits, sim tools.SimExecutor) *Agent {
	t.Helper()
	led, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(prov, budget.New(lim), govLimits, led, sim, Config{Model: prov.Model()})
}

func TestSubagentModelRouting(t *testing.T) {
	a := newTestAgent(t,
		provider.NewMock("mock", provider.NormalScript(), false),
		governor.DefaultLimits(),
		budget.Limits{MaxTokens: 1_000_000, MaxUSD: 100},
		tools.SimExecutor{})
	a.cfg.MaxSubagentDepth = 2
	a.cfg.SubagentModel = "cheap-model"

	child := a.newChild(budget.Limits{MaxTokens: 1000, MaxUSD: 1})
	if child.Model() != "cheap-model" {
		t.Fatalf("subagent should route to the configured model, got %q", child.Model())
	}
	if a.Model() == "cheap-model" {
		t.Fatal("the parent model must be unchanged by routing")
	}
	// A nested subagent inherits the routed model too.
	grand := child.newChild(budget.Limits{MaxTokens: 100, MaxUSD: 1})
	if grand.Model() != "cheap-model" {
		t.Fatalf("nested subagent should stay on the routed model, got %q", grand.Model())
	}
}

func TestSubagentNoRoutingByDefault(t *testing.T) {
	a := newTestAgent(t,
		provider.NewMock("mock", provider.NormalScript(), false),
		governor.DefaultLimits(),
		budget.Limits{MaxTokens: 1_000_000, MaxUSD: 100},
		tools.SimExecutor{})
	a.cfg.MaxSubagentDepth = 2
	child := a.newChild(budget.Limits{MaxTokens: 1000, MaxUSD: 1})
	if child.Model() != a.Model() {
		t.Fatalf("without SubagentModel, a child should share the parent model: %q vs %q", child.Model(), a.Model())
	}
}

func TestNormalTaskCompletes(t *testing.T) {
	a := newTestAgent(t,
		provider.NewMock("mock", provider.NormalScript(), false),
		governor.DefaultLimits(),
		budget.Limits{MaxTokens: 1_000_000, MaxUSD: 100},
		tools.SimExecutor{})
	o, err := a.Run(context.Background(), "do it")
	if err != nil {
		t.Fatal(err)
	}
	if o.Stop != StopCompleted {
		t.Fatalf("expected completed, got %s (%s)", o.Stop, o.Reason)
	}
}

func TestRunawayIsHaltedByGovernor(t *testing.T) {
	a := newTestAgent(t,
		provider.NewMock("mock", provider.RunawayScript(), true),
		governor.Limits{RepetitionWindow: 8, RepetitionThreshold: 3, MaxTurns: 1000},
		budget.Limits{MaxTokens: 1_000_000_000, MaxUSD: 1_000_000},
		tools.SimExecutor{FailEdits: true})
	o, err := a.Run(context.Background(), "loop forever")
	if err != nil {
		t.Fatal(err)
	}
	if o.Stop != "repetition" {
		t.Fatalf("expected repetition halt, got %s", o.Stop)
	}
	if o.Turns > 10 {
		t.Fatalf("runaway should be stopped quickly, took %d turns", o.Turns)
	}
}

func TestTranscriptValidAfterMidLoopHalt(t *testing.T) {
	// A runaway trips the repetition breaker mid tool-loop. The transcript must
	// still end with a complete tool_results message (one result per tool_use),
	// or a follow-up turn would be rejected by the provider.
	a := newTestAgent(t,
		provider.NewMock("mock", provider.RunawayScript(), true),
		governor.Limits{RepetitionWindow: 8, RepetitionThreshold: 3, MaxTurns: 1000},
		budget.Limits{MaxTokens: 1_000_000_000, MaxUSD: 1_000_000},
		tools.SimExecutor{FailEdits: true})
	o, err := a.Run(context.Background(), "loop")
	if err != nil {
		t.Fatal(err)
	}
	if o.Stop != "repetition" {
		t.Fatalf("want repetition halt, got %s", o.Stop)
	}

	calls, results := 0, 0
	for _, m := range a.messages {
		calls += len(m.ToolCalls)
		results += len(m.ToolResults)
	}
	if results < calls {
		t.Fatalf("dangling tool_use: %d tool calls but only %d results", calls, results)
	}
	last := a.messages[len(a.messages)-1]
	if last.Role != "user" || len(last.ToolResults) == 0 {
		t.Fatalf("transcript should end with a tool_results user message, got role=%q", last.Role)
	}
}

func TestBudgetStopsExpensiveRun(t *testing.T) {
	a := newTestAgent(t,
		provider.NewMock("claude-sonnet-4-6", provider.HeavyScript(), true),
		governor.Limits{MaxTurns: 1000},
		budget.Limits{MaxUSD: 0.50, MaxTokens: 1_000_000_000},
		tools.SimExecutor{})
	o, err := a.Run(context.Background(), "burn money")
	if err != nil {
		t.Fatal(err)
	}
	if o.Stop != StopBudget {
		t.Fatalf("expected budget stop, got %s (%s)", o.Stop, o.Reason)
	}
}
