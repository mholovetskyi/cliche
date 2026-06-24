package tui

import (
	"strings"
	"testing"
)

func TestBoxDimensions(t *testing.T) {
	b := box("title", []string{"a", "b"}, 20, 5)
	if len(b) != 5 {
		t.Fatalf("box height = %d, want 5", len(b))
	}
	if !strings.Contains(b[0], "title") {
		t.Fatalf("top border should inlay the title: %q", b[0])
	}
}

func TestRenderDashboardComposesPanes(t *testing.T) {
	status := []string{"mode suggest", "spend $0.01"}
	tasks := []string{"[ ] do the thing"}
	changes := []string{"~ main.go", "+ new.go"}
	frame := renderDashboard(80, 16, status, tasks, changes)

	if len(frame) != 16 {
		t.Fatalf("dashboard height = %d, want 16", len(frame))
	}
	joined := strings.Join(frame, "\n")
	for _, want := range []string{"trust", "tasks", "changes", "mode suggest", "do the thing", "main.go", "new.go"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("dashboard missing %q:\n%s", want, joined)
		}
	}
	// Two columns side by side → a content row carries both panes' borders.
	if !strings.Contains(joined, "│") {
		t.Fatal("panes should have borders")
	}
}
