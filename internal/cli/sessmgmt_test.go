package cli

import (
	"bytes"
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
	"github.com/mholovetskyi/cliche/internal/style"
	"github.com/mholovetskyi/cliche/internal/tools"
)

func newMgmtSession(t *testing.T, dir string, out *bytes.Buffer) *session {
	t.Helper()
	led, _ := ledger.Open(t.TempDir())
	a := agent.New(
		provider.NewMock("mock", provider.NormalScript(), false),
		budget.New(budget.Limits{MaxTokens: 1_000_000, MaxUSD: 100}),
		governor.DefaultLimits(),
		led, tools.SimExecutor{}, agent.Config{Model: "mock"},
	)
	return &session{a: a, out: out, dir: dir, cfg: config.Config{Provider: "mock"}, created: time.Now()}
}

func TestSessionManagement(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	s := newMgmtSession(t, dir, &out)

	// Seed a saved session on disk to resume into.
	saved := sess.Record{ID: "20200101-000000", Title: "earlier work", Created: time.Now(),
		Messages: []provider.Message{{Role: "user", Text: "hi"}}}
	if err := sess.Save(dir, saved); err != nil {
		t.Fatal(err)
	}

	// /new gives a fresh id and clears the transcript title.
	s.id, s.title = "cur", "current work"
	s.newSession()
	if s.id == "cur" || s.title != "" {
		t.Fatalf("/new should rotate to a fresh empty session, got id=%q title=%q", s.id, s.title)
	}

	// /sessions lists the saved one.
	out.Reset()
	s.showSessions()
	if !strings.Contains(out.String(), "20200101-000000") || !strings.Contains(out.String(), "earlier work") {
		t.Fatalf("/sessions should list saved sessions:\n%s", out.String())
	}

	// /resume loads it (transcript swapped; id adopted).
	out.Reset()
	s.resumeSession("/resume 20200101-000000")
	if s.id != "20200101-000000" || s.title != "earlier work" {
		t.Fatalf("/resume should adopt the saved session, got id=%q title=%q", s.id, s.title)
	}
	if len(s.a.Transcript()) != 1 {
		t.Fatalf("/resume should restore the transcript, got %d messages", len(s.a.Transcript()))
	}
	if !strings.Contains(out.String(), "resumed") {
		t.Fatalf("/resume should confirm:\n%s", out.String())
	}

	// Resuming an unknown id reports it rather than crashing.
	out.Reset()
	s.resumeSession("/resume nope-123")
	if !strings.Contains(out.String(), "resume:") {
		t.Fatalf("/resume of a missing id should report an error:\n%s", out.String())
	}
}

func TestInputBar(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = true, false
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	var out bytes.Buffer
	s := newMgmtSession(t, t.TempDir(), &out)

	top := s.barTop()
	if !strings.Contains(top, "╭") || !strings.Contains(top, "╮") {
		t.Fatalf("barTop should be a framed border: %q", top)
	}
	if style.Width(top) != barWidth {
		t.Fatalf("barTop width = %d, want %d (frame skewed): %q", style.Width(top), barWidth, top)
	}
	if !strings.Contains(s.barPrompt(), "❯") || !strings.Contains(s.barPrompt(), "│") {
		t.Fatalf("barPrompt should carry the left border + chevron: %q", s.barPrompt())
	}
}
