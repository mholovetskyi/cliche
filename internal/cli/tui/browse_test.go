package tui

import (
	"strings"
	"testing"
)

func TestRenderFrameStructure(t *testing.T) {
	items := []Item{
		{Label: "alpha", Preview: []string{"alpha details", "line two"}},
		{Label: "beta", Preview: []string{"beta details"}},
		{Label: "gamma", Preview: []string{"gamma details"}},
	}
	l := NewList(len(items))
	lines := renderFrame(l, items, 60, 8, "HEADER", "open")

	if len(lines) != 8 {
		t.Fatalf("frame should be exactly height (8) lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "HEADER") {
		t.Fatalf("header line missing the header text:\n%q", lines[0])
	}
	// Selected (alpha) row is marked, and its preview shows on the right.
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "▸") || !strings.Contains(joined, "alpha") {
		t.Fatalf("selected row should be marked with ▸ alpha:\n%s", joined)
	}
	if !strings.Contains(joined, "alpha details") {
		t.Fatalf("right pane should show the selected item's preview:\n%s", joined)
	}
	if !strings.Contains(lines[len(lines)-1], "quit") {
		t.Fatalf("footer should list keys:\n%q", lines[len(lines)-1])
	}
}

func TestRenderFramePreviewFollowsSelection(t *testing.T) {
	items := []Item{
		{Label: "a", Preview: []string{"PREVIEW_A"}},
		{Label: "b", Preview: []string{"PREVIEW_B"}},
	}
	l := NewList(len(items))
	l.Down() // select b
	out := strings.Join(renderFrame(l, items, 50, 6, "h", "open"), "\n")
	if !strings.Contains(out, "PREVIEW_B") || strings.Contains(out, "PREVIEW_A") {
		t.Fatalf("preview should track the selection (expected B, not A):\n%s", out)
	}
}
