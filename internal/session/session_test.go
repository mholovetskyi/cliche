package session

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mholovetskyi/cliche/internal/budget"
	"github.com/mholovetskyi/cliche/internal/provider"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	rec := Record{
		ID:       NewID(time.Now()),
		Title:    "fix the build",
		Provider: "openrouter",
		Model:    "openai/gpt-4o-mini",
		Created:  time.Now().UTC().Truncate(time.Second),
		Updated:  time.Now().UTC().Truncate(time.Second),
		Usage:    budget.Usage{InputTokens: 1200, OutputTokens: 300, USD: 0.004},
		Messages: []provider.Message{
			{Role: "user", Text: "do it"},
			{Role: "assistant", Text: "ok", ToolCalls: []provider.ToolCall{{
				ID: "t1", Name: "edit_file",
				Args: map[string]string{"file": "x.go", "replace_all": "true"},
				Raw:  json.RawMessage(`{"file":"x.go","replace_all":true}`),
			}}},
			{Role: "user", ToolResults: []provider.ToolResult{{ID: "t1", Content: "edited"}}},
		},
	}
	if err := Save(root, rec); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != rec.Title || got.Model != rec.Model || len(got.Messages) != 3 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.Usage.USD != rec.Usage.USD {
		t.Fatalf("usage not persisted: %+v", got.Usage)
	}
	// The crucial bit: ToolCall.Raw must survive with TYPES intact so resume
	// echoes faithful tool inputs (replace_all stays a boolean, not "true").
	var parsed map[string]any
	if err := json.Unmarshal(got.Messages[1].ToolCalls[0].Raw, &parsed); err != nil {
		t.Fatalf("persisted Raw is not valid JSON: %v", err)
	}
	if parsed["replace_all"] != true || parsed["file"] != "x.go" {
		t.Fatalf("ToolCall.Raw lost its typed values across persistence: %+v", parsed)
	}
}

// Per-session Trust-Kernel caps survive a save/load round-trip (so a budget dialed
// for one chat is restored when it's reopened); absence stays nil (config defaults).
func TestLimitsRoundTrip(t *testing.T) {
	root := t.TempDir()
	rec := Record{ID: NewID(time.Now()), Title: "dialed", Created: time.Now(), Updated: time.Now(),
		Limits: &Limits{MaxUSD: 25, MaxTokens: 0, MaxTurns: 99}}
	if err := Save(root, rec); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Limits == nil || got.Limits.MaxUSD != 25 || got.Limits.MaxTurns != 99 {
		t.Fatalf("limits not persisted: %+v", got.Limits)
	}
	// A record with no limits stays nil (→ the serve uses config defaults).
	bare := Record{ID: NewID(time.Now().Add(time.Second)), Created: time.Now(), Updated: time.Now()}
	_ = Save(root, bare)
	if g, _ := Load(root, bare.ID); g.Limits != nil {
		t.Fatalf("absent limits should load as nil, got %+v", g.Limits)
	}
}

func TestListAndLatest(t *testing.T) {
	root := t.TempDir()
	if l := Latest(root); l != "" {
		t.Fatalf("no sessions yet should give empty latest, got %q", l)
	}
	older := Record{ID: "20260101-000000", Title: "old", Updated: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	newer := Record{ID: "20260622-120000", Title: "new", Updated: time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)}
	if err := Save(root, older); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, newer); err != nil {
		t.Fatal(err)
	}
	metas, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 || metas[0].ID != newer.ID {
		t.Fatalf("List should be newest-first: %+v", metas)
	}
	if Latest(root) != newer.ID {
		t.Fatalf("Latest should be the newest session, got %q", Latest(root))
	}
}

func TestLoadMissing(t *testing.T) {
	if _, err := Load(t.TempDir(), "nope"); err == nil {
		t.Fatal("loading a missing session should error")
	}
}
