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

func TestShortModelAndPct(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"openai/gpt-4o-mini", "gpt-4o-mini"},
		{"anthropic/claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"llama3.1", "llama3.1"},
	} {
		if got := shortModel(tc.in); got != tc.want {
			t.Errorf("shortModel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	for _, tc := range []struct {
		part, whole, want int
	}{{0, 100, 0}, {50, 100, 50}, {1, 3, 33}, {200, 100, 100}, {5, 0, 0}} {
		if got := pctOf(tc.part, tc.whole); got != tc.want {
			t.Errorf("pctOf(%d,%d) = %d, want %d", tc.part, tc.whole, got, tc.want)
		}
	}
}

func TestCompactHeaderContent(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = false, true // plain mode: assert the text survives
	defer func() { style.Enabled, noColor = oldE, oldNC }()
	h := compactHeader("openrouter", "gpt-4o-mini", "suggest", "env")
	// The mode now rides in a badge (uppercased); under NO_COLOR it degrades to
	// [SUGGEST] but the state still reads.
	for _, want := range []string{"cliché", "openrouter", "gpt-4o-mini", "SUGGEST", "key env"} {
		if !strings.Contains(h, want) {
			t.Errorf("compact header missing %q: %q", want, h)
		}
	}
}

func TestChevronColorBiasesToRedNearCaps(t *testing.T) {
	// By mode when there's headroom…
	if chevronColor(modeSuggest, 0.1, 0.1) != style.GrayRGB {
		t.Error("suggest with headroom should be gray")
	}
	if chevronColor(modeFull, 0, 0) != style.RedRGB {
		t.Error("full mode should be red")
	}
	// …but red once budget OR context crosses 80%, regardless of mode.
	if chevronColor(modeSuggest, 0.85, 0) != style.RedRGB {
		t.Error("high budget use should force the chevron red")
	}
	if chevronColor(modePlan, 0, 0.9) != style.RedRGB {
		t.Error("high context use should force the chevron red even in plan mode")
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
