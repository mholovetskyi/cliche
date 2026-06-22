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
