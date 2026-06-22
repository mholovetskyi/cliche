package cli

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/budget"
	"github.com/mholovetskyi/cliche/internal/governor"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/tools"
)

func TestSessionLoop(t *testing.T) {
	led, err := ledger.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := agent.New(
		provider.NewMock("mock", provider.NormalScript(), false),
		budget.New(budget.Limits{MaxTokens: 1_000_000, MaxUSD: 100}),
		governor.DefaultLimits(),
		led,
		tools.SimExecutor{},
		agent.Config{Model: "mock"},
	)
	var out bytes.Buffer
	a.SetObserver(func(e agent.Event) { printEvent(&out, e) })

	s := &session{
		a:   a,
		r:   bufio.NewReader(strings.NewReader("fix the bug\n/cost\n/exit\n")),
		out: &out,
		dir: t.TempDir(),
	}
	if code := s.loop(); code != 0 {
		t.Fatalf("session exit code = %d, want 0", code)
	}

	got := out.String()
	for _, want := range []string{"done", "session:", "bye."} {
		if !strings.Contains(got, want) {
			t.Fatalf("session output missing %q:\n%s", want, got)
		}
	}
}

func TestSessionClearAndUnknownSlash(t *testing.T) {
	led, _ := ledger.Open(t.TempDir())
	a := agent.New(
		provider.NewMock("mock", provider.NormalScript(), false),
		budget.New(budget.Limits{MaxTokens: 1_000_000}),
		governor.DefaultLimits(),
		led, tools.SimExecutor{}, agent.Config{Model: "mock"},
	)
	var out bytes.Buffer
	s := &session{a: a, r: bufio.NewReader(strings.NewReader("/clear\n/bogus\n/exit\n")), out: &out, dir: t.TempDir()}
	s.loop()
	got := out.String()
	if !strings.Contains(got, "context cleared") {
		t.Fatalf("expected /clear output:\n%s", got)
	}
	if !strings.Contains(got, "unknown command") {
		t.Fatalf("expected unknown-command output:\n%s", got)
	}
}
