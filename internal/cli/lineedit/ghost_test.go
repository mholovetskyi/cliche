package lineedit

import "testing"

func TestHistorySuggest(t *testing.T) {
	h := NewHistory([]string{"fix the auth bug", "run the tests", "fix the build"})
	// Most-recent distinct extension wins.
	if got := h.Suggest("fix the "); got != "build" {
		t.Fatalf("Suggest(%q) = %q, want %q", "fix the ", got, "build")
	}
	if got := h.Suggest("run "); got != "the tests" {
		t.Fatalf("Suggest(%q) = %q, want %q", "run ", got, "the tests")
	}
	if got := h.Suggest("nope"); got != "" {
		t.Fatalf("Suggest no-match = %q, want empty", got)
	}
	if got := h.Suggest(""); got != "" {
		t.Fatalf("Suggest empty prefix = %q, want empty", got)
	}
}

// Right-arrow at end-of-line accepts the inline autosuggestion.
func TestEditorAcceptsGhostSuggestion(t *testing.T) {
	hist := NewHistory([]string{"fix the failing test"})
	// type "fix" → ghost "the failing test" shown → Right accepts → Enter submits.
	line, err := runEditor(t, "fix\x1b[C\r", hist)
	if err != nil || line != "fix the failing test" {
		t.Fatalf("ghost accept = %q (%v), want the full suggestion", line, err)
	}
}

// Right-arrow with no suggestion (and cursor already at end) is a harmless no-op.
func TestEditorRightNoSuggestionNoOp(t *testing.T) {
	line, err := runEditor(t, "abc\x1b[C\r", nil)
	if err != nil || line != "abc" {
		t.Fatalf("right with no ghost = %q (%v), want abc", line, err)
	}
}
