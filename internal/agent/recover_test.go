package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mholovetskyi/cliche/internal/budget"
	"github.com/mholovetskyi/cliche/internal/governor"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/tools"
)

type panicProvider struct{}

func (panicProvider) Name() string  { return "panic" }
func (panicProvider) Model() string { return "mock" }
func (panicProvider) Complete(context.Context, provider.Request) (provider.Response, error) {
	panic("kaboom in the provider")
}

// A panic anywhere in a turn must surface as a structured error, never crash the
// process (which would take down a long-running serve/cron/telegram).
func TestRunRecoversFromPanic(t *testing.T) {
	led, _ := ledger.Open(t.TempDir())
	a := New(panicProvider{}, budget.New(budget.Limits{MaxTokens: 1_000_000}), governor.DefaultLimits(), led, tools.SimExecutor{}, Config{Model: "mock"})
	o, err := a.Run(context.Background(), "go")
	if err == nil {
		t.Fatal("a provider panic must surface as an error, not crash")
	}
	if o.Stop != StopError || !strings.Contains(err.Error(), "recovered") {
		t.Fatalf("expected a recovered StopError, got stop=%q err=%v", o.Stop, err)
	}
}

type blockingProvider struct{}

func (blockingProvider) Name() string  { return "block" }
func (blockingProvider) Model() string { return "mock" }
func (blockingProvider) Complete(ctx context.Context, _ provider.Request) (provider.Response, error) {
	<-ctx.Done()
	return provider.Response{}, ctx.Err()
}

// MaxWallClock must bound a run even if a single completion hangs.
func TestRunHonorsWallClock(t *testing.T) {
	led, _ := ledger.Open(t.TempDir())
	a := New(blockingProvider{}, budget.New(budget.Limits{MaxTokens: 1_000_000}), governor.DefaultLimits(), led, tools.SimExecutor{}, Config{Model: "mock", MaxWallClock: 150 * time.Millisecond})
	done := make(chan struct{})
	go func() { a.Run(context.Background(), "go"); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("wall-clock did not bound a hung completion")
	}
}
