package lineedit

import "testing"

func TestHistoryNavigation(t *testing.T) {
	h := NewHistory([]string{"one", "two"})
	if got := h.Prev("wip"); got != "two" {
		t.Fatalf("Prev #1 = %q, want two", got)
	}
	if got := h.Prev("two"); got != "one" {
		t.Fatalf("Prev #2 = %q, want one", got)
	}
	if got := h.Prev("one"); got != "one" {
		t.Fatalf("Prev at oldest = %q, want one (stay)", got)
	}
	if got := h.Next(); got != "two" {
		t.Fatalf("Next #1 = %q, want two", got)
	}
	if got := h.Next(); got != "wip" {
		t.Fatalf("Next should restore scratch = %q, want wip", got)
	}
	if got := h.Next(); got != "wip" {
		t.Fatalf("Next at newest = %q, want wip", got)
	}
}

func TestHistoryAddDedup(t *testing.T) {
	h := NewHistory(nil)
	h.Add("a")
	h.Add("a") // consecutive dup, suppressed
	h.Add("b")
	h.Add("") // empty, skipped
	if got := h.Prev(""); got != "b" {
		t.Fatalf("Prev = %q, want b", got)
	}
	if got := h.Prev(""); got != "a" {
		t.Fatalf("Prev = %q, want a", got)
	}
	if got := h.Prev(""); got != "a" {
		t.Fatalf("Prev oldest = %q, want a", got)
	}
}

func TestHistoryEmpty(t *testing.T) {
	h := NewHistory(nil)
	if got := h.Prev("x"); got != "x" {
		t.Fatalf("empty Prev = %q, want x", got)
	}
	if got := h.Next(); got != "" {
		t.Fatalf("empty Next = %q, want ''", got)
	}
}
