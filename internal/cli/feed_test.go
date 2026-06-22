package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/style"
)

// TestFeedColumnAlignment proves the target column lands at a fixed display
// offset no matter the verb length or the color escapes — the whole point of the
// Pad'd verb column (and the bug the old emoji glyphs caused).
func TestFeedColumnAlignment(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = true, false
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	const prefix = 2 + 1 + 1 + 6 + 1 // indent + bullet + space + Pad(verb,6) + space
	for _, tc := range []struct{ tool, detail string }{
		{"read_file", "main.go"},
		{"search_files", "TODO"},         // longest verb (Search)
		{"run_command", "go test ./..."}, // shortest verb (Run)
		{"edit_file", "internal/x/у.go"}, // a non-ASCII rune in the target
	} {
		var buf bytes.Buffer
		printEvent(&buf, agent.Event{Kind: "tool_call", Tool: tc.tool, Detail: tc.detail})
		line := strings.TrimRight(buf.String(), "\n")
		if got, want := style.Width(line), prefix+style.Width(tc.detail); got != want {
			t.Errorf("tool %s: line width %d, want %d — column drifted: %q", tc.tool, got, want, line)
		}
	}
}

func TestFeedResultMarkers(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = true, false
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	var ok, fail bytes.Buffer
	printEvent(&ok, agent.Event{Kind: "tool_result", Tool: "read_file", OK: true, Detail: "42 lines"})
	printEvent(&fail, agent.Event{Kind: "tool_result", Tool: "run_command", OK: false, Detail: "exit 1"})
	if !strings.Contains(ok.String(), "✓") || !strings.Contains(ok.String(), "42 lines") {
		t.Fatalf("success result should show a tick + detail: %q", ok.String())
	}
	if !strings.Contains(fail.String(), "✗") || !strings.Contains(fail.String(), "exit 1") {
		t.Fatalf("failure result should show a cross + detail: %q", fail.String())
	}
}
