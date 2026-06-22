package cli

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"time"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/budget"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/governor"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/provider"
	sess "github.com/mholovetskyi/cliche/internal/session"
	"github.com/mholovetskyi/cliche/internal/tools"
)

func TestSessionPersistsAndResumes(t *testing.T) {
	dir := t.TempDir()
	led, _ := ledger.Open(t.TempDir())
	a := agent.New(
		provider.NewMock("mock", provider.NormalScript(), false),
		budget.New(budget.Limits{MaxTokens: 1_000_000}),
		governor.DefaultLimits(),
		led, tools.SimExecutor{}, agent.Config{Model: "mock"},
	)
	var out bytes.Buffer
	s := &session{
		a: a, r: bufio.NewReader(strings.NewReader("fix the bug\n/exit\n")),
		out: &out, dir: dir, cfg: config.Config{Provider: "mock", Model: "mock"},
		id: sess.NewID(time.Now()), created: time.Now(),
	}
	a.SetObserver(func(agent.Event) {})
	s.loop()

	metas, err := sess.List(dir)
	if err != nil || len(metas) != 1 {
		t.Fatalf("expected 1 persisted session, got %d (err=%v)", len(metas), err)
	}
	if metas[0].Title != "fix the bug" {
		t.Fatalf("session title should be the first prompt, got %q", metas[0].Title)
	}
	rec, err := sess.Load(dir, metas[0].ID)
	if err != nil || len(rec.Messages) == 0 {
		t.Fatalf("persisted session should carry the transcript: %d msgs, err=%v", len(rec.Messages), err)
	}
}

func TestSessionSwitchModel(t *testing.T) {
	led, _ := ledger.Open(t.TempDir())
	a := agent.New(
		provider.NewMock("mock", provider.NormalScript(), false),
		budget.New(budget.Limits{MaxTokens: 1_000_000}),
		governor.DefaultLimits(),
		led, tools.SimExecutor{}, agent.Config{Model: "claude-sonnet-4-6"},
	)
	var out bytes.Buffer
	s := &session{a: a, out: &out, cfg: config.Config{Provider: "openrouter"}}

	s.switchModel("/model") // no arg → show current
	if !strings.Contains(out.String(), "claude-sonnet-4-6") {
		t.Fatalf("/model should show the current model:\n%s", out.String())
	}
	out.Reset()
	s.switchModel("/model anthropic/claude-opus-4.8")
	if a.Model() != "anthropic/claude-opus-4.8" {
		t.Fatalf("/model <id> should switch the agent model, got %q", a.Model())
	}
	if !strings.Contains(out.String(), "anthropic/claude-opus-4.8") {
		t.Fatalf("/model switch should be acknowledged:\n%s", out.String())
	}
}

func TestSessionDiffAndUndo(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "a.txt")
	if err := os.WriteFile(file, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	j := tools.NewEditJournal(root)
	e := tools.OSExecutor{Root: root, Policy: tools.Policy{Yolo: true}, Journal: j}
	if r := e.Execute(context.Background(), "edit_file", map[string]string{"file": "a.txt", "old_string": "old", "new_string": "new"}); !r.Success {
		t.Fatalf("setup edit failed: %s", r.Output)
	}

	var out bytes.Buffer
	s := &session{out: &out, journal: j, dir: root}

	s.showDiff()
	if got := out.String(); !strings.Contains(got, "a.txt") || !strings.Contains(got, "- old") || !strings.Contains(got, "+ new") {
		t.Fatalf("/diff output should show the change:\n%s", got)
	}

	out.Reset()
	s.undo()
	if !strings.Contains(out.String(), "reverted a.txt") {
		t.Fatalf("/undo should report the reverted file:\n%s", out.String())
	}
	if got, _ := os.ReadFile(file); string(got) != "old\n" {
		t.Fatalf("/undo should restore the original content, got %q", got)
	}

	// With nothing changed, /diff says so and /undo has nothing to do.
	out.Reset()
	s.showDiff()
	if !strings.Contains(out.String(), "no file changes") {
		t.Fatalf("/diff with no changes should say so:\n%s", out.String())
	}
}

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
