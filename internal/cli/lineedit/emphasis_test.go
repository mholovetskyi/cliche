package lineedit

import (
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/style"
)

func TestSlashMenuCapturesMatchPositions(t *testing.T) {
	m := newSlashMenu([]Command{{Name: "/status"}, {Name: "/models"}})
	m.update("/st")
	if len(m.matchPos) != len(m.filtered) {
		t.Fatalf("matchPos (%d) must track filtered (%d)", len(m.matchPos), len(m.filtered))
	}
	// "/st" matches '/','s','t' at indices 0,1,2 of "/status".
	if m.filtered[0].Name != "/status" {
		t.Fatalf("/status should rank first, got %q", m.filtered[0].Name)
	}
	got := m.matchPos[0]
	if len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 2 {
		t.Fatalf("match positions = %v, want [0 1 2]", got)
	}
}

func TestEmphasizeMatchPreservesWidth(t *testing.T) {
	old := style.Enabled
	defer func() { style.Enabled = old }()

	style.Enabled = true
	s := emphasizeMatch("/status", []int{0, 1, 2}, false)
	if style.Width(s) != style.Width("/status") {
		t.Fatalf("emphasize changed visible width: %d vs %d", style.Width(s), style.Width("/status"))
	}
	if !strings.Contains(s, "\x1b[1m") {
		t.Fatal("matched runes should be bold")
	}
	style.Enabled = false
	if got := emphasizeMatch("/status", []int{0}, false); got != "/status" {
		t.Fatalf("disabled should be plain, got %q", got)
	}
}
