package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/style"
)

func TestClosestCommand(t *testing.T) {
	cases := map[string]string{
		"/exi":   "/exit",   // unique prefix
		"/mod":   "/mode",   // ambiguous prefix (/mode, /model) → nearest by edit distance
		"/verfy": "/verify", // edit distance 1 (missing letter)
		"/xyz":   "",        // nothing close enough — don't guess wildly
	}
	for in, want := range cases {
		if got := closestCommand(in); got != want {
			t.Errorf("closestCommand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandPrefixAndTip(t *testing.T) {
	cases := map[string]string{
		"/st":   "/status", // unique prefixes expand…
		"/di":   "/diff",
		"/u":    "/undo",
		"/h":    "/help",
		"/cost": "/cost", // a full name is its own unique prefix
		"/s":    "",      // …ambiguous ones do not (status|sessions)
		"/c":    "",      // cost|context|clear|commit
		"/r":    "",      // rules|rewind|recover|resume
		"/m":    "",      // model|models|mode
		"/zzz":  "",
	}
	for in, want := range cases {
		if got := expandPrefix(in); got != want {
			t.Errorf("expandPrefix(%q) = %q, want %q", in, got, want)
		}
	}
	// Tips: silent on the first prompt and in the gaps, rotating on the interval.
	if promptTip(0) != "" || promptTip(promptTipEvery-1) != "" {
		t.Error("promptTip should be silent on the first prompt and between rotations")
	}
	if promptTip(promptTipEvery) != promptTips[0] {
		t.Error("promptTip should show the first tip at the first interval")
	}
	if promptTip(2*promptTipEvery) != promptTips[1] {
		t.Error("promptTip should rotate to the next tip")
	}
}

func TestSlashRunsAbbreviationAndDisambiguates(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = false, true
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	// /h expands to /help and executes (help lists every command).
	var out bytes.Buffer
	s := &session{out: &out}
	if s.slash("/h") {
		t.Fatal("/h should not exit the session")
	}
	if !strings.Contains(out.String(), "/status") {
		t.Fatalf("/h should run /help and list commands:\n%s", out.String())
	}

	// /c is ambiguous → a filtered command palette of the candidates, no execution.
	out.Reset()
	s.slash("/c")
	if got := out.String(); !strings.Contains(got, "/cost") || !strings.Contains(got, "/clear") {
		t.Fatalf("/c should open a filtered command palette:\n%s", got)
	}
}

func TestCommandPalette(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = false, true
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	var out bytes.Buffer
	s := &session{out: &out}

	// The full palette lists every command and the controls footer.
	s.commandPalette("")
	all := out.String()
	for _, want := range []string{"commands", "/status", "/kill", "Ctrl-C", "@ attach"} {
		if !strings.Contains(all, want) {
			t.Errorf("full palette missing %q:\n%s", want, all)
		}
	}

	// A prefix filters the dropdown to matching commands.
	out.Reset()
	s.commandPalette("/c")
	f := out.String()
	if !strings.Contains(f, "/cost") || !strings.Contains(f, "/clear") || !strings.Contains(f, "/commit") {
		t.Fatalf("filtered palette should show the /c commands:\n%s", f)
	}
	if strings.Contains(f, "/status") || strings.Contains(f, "/kill") {
		t.Fatalf("filtered palette should exclude non-matches:\n%s", f)
	}

	// No match is reported, not an empty box.
	out.Reset()
	s.commandPalette("/zzz")
	if !strings.Contains(out.String(), "no command matches") {
		t.Fatalf("a non-matching filter should say so:\n%s", out.String())
	}
}

func TestHelpListsEveryCommand(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = false, true
	defer func() { style.Enabled, noColor = oldE, oldNC }()
	var out bytes.Buffer
	s := &session{out: &out}
	s.help()
	got := out.String()
	for _, c := range slashCommands {
		if !strings.Contains(got, c.name) {
			t.Errorf("/help is missing %q:\n%s", c.name, got)
		}
	}
	// The startup hint draws from the same table, so it can't drift.
	if !strings.Contains(slashHint(), "/cost") || !strings.Contains(slashHint(), "/exit") {
		t.Errorf("slashHint should list commands: %q", slashHint())
	}
}
