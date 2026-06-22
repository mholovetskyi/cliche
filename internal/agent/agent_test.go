package agent

import (
	"context"
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
