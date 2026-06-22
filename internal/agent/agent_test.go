package agent

import (
	"context"
	"testing"

	"github.com/mholovetskyi/cliche/internal/budget"
	"github.com/mholovetskyi/cliche/internal/governor"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/tools"
)

func newTestAgent(t *testing.T, prov provider.Provider, gov *governor.Governor, lim budget.Limits, sim tools.SimExecutor) *Agent {
	t.Helper()
	led, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(prov, budget.New(lim), gov, led, sim, Config{Model: prov.Model()})
}

func TestNormalTaskCompletes(t *testing.T) {
	a := newTestAgent(t,
		provider.NewMock("mock", provider.NormalScript(), false),
		governor.New(governor.DefaultLimits()),
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
		governor.New(governor.Limits{RepetitionWindow: 8, RepetitionThreshold: 3, MaxTurns: 1000}),
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

func TestBudgetStopsExpensiveRun(t *testing.T) {
	a := newTestAgent(t,
		provider.NewMock("claude-sonnet-4-6", provider.HeavyScript(), true),
		governor.New(governor.Limits{MaxTurns: 1000}),
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
