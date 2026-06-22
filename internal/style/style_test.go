package style

import (
	"strings"
	"testing"
)

func TestWrapRespectsEnabled(t *testing.T) {
	old := Enabled
	defer func() { Enabled = old }()

	Enabled = false
	if got := Red("hi"); got != "hi" {
		t.Fatalf("disabled styling should be plain, got %q", got)
	}

	Enabled = true
	got := Red("hi")
	if !strings.Contains(got, "hi") || !strings.HasPrefix(got, "\x1b[") || !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("enabled styling should wrap with ANSI, got %q", got)
	}

	if Red("") != "" {
		t.Fatal("empty input should stay empty")
	}
}
