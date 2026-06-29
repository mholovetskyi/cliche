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
	ex := OSExecutor{Root: t.TempDir()}
	res := ex.Execute(context.Background(), "remember_user", map[string]string{"fact": "prefers TypeScript"})
	if !res.Success {
		t.Fatalf("remember_user failed: %s", res.Output)
	}
	if !strings.Contains(profile.Load(), "prefers TypeScript") {
		t.Fatal("fact not written to the global profile")
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
