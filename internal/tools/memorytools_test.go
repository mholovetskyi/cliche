package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mholovetskyi/cliche/internal/profile"
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/session"
)

func TestRememberUserTool(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	// Memory writes are approval-gated — an approving user lets it through.
	ok := &spyApprover{allow: true}
	ex := OSExecutor{Root: t.TempDir(), Approve: ok.approve}
	res := ex.Execute(context.Background(), "remember_user", map[string]string{"fact": "prefers TypeScript"})
	if !res.Success {
		t.Fatalf("remember_user failed: %s", res.Output)
	}
	if ok.calls == 0 || ok.lastAction != "write" {
		t.Fatalf("remember_user should ask the approver as a write; calls=%d action=%q", ok.calls, ok.lastAction)
	}
	if !strings.Contains(profile.Load(), "prefers TypeScript") {
		t.Fatal("fact not written to the global profile")
	}
}

// The Trust-Kernel win: a declining user blocks the memory write (no silent persist).
func TestRememberApprovalGate(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	deny := &spyApprover{allow: false}
	ex := OSExecutor{Root: root, Approve: deny.approve}
	if res := ex.Execute(context.Background(), "remember", map[string]string{"fact": "x"}); res.Success {
		t.Fatal("remember should be blocked when the user declines")
	}
	if res := ex.Execute(context.Background(), "remember_user", map[string]string{"fact": "y"}); res.Success {
		t.Fatal("remember_user should be blocked when the user declines")
	}
	if strings.Contains(profile.Load(), "y") {
		t.Fatal("a declined fact must not be written to the profile")
	}
}

func TestRecallTool(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	_ = session.Save(root, session.Record{
		ID: "20250101-000000", Title: "db layer", Created: now, Updated: now,
		Messages: []provider.Message{{Role: "assistant", Text: "we chose Postgres with a connection pool"}},
	})
	ex := OSExecutor{Root: root}
	res := ex.Execute(context.Background(), "recall", map[string]string{"query": "postgres"})
	if !res.Success || !strings.Contains(strings.ToLower(res.Output), "postgres") {
		t.Fatalf("recall didn't surface the match: %q", res.Output)
	}
	// A miss is a successful no-op, not an error.
	if res := ex.Execute(context.Background(), "recall", map[string]string{"query": "kubernetes"}); !res.Success {
		t.Fatalf("recall miss should succeed: %s", res.Output)
	}
}
