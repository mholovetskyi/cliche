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

func TestColorAt(t *testing.T) {
	stops := []RGB{{0, 0, 0}, {10, 20, 30}}
	if c := colorAt(stops, 0); c != (RGB{0, 0, 0}) {
		t.Fatalf("t=0 should be the first stop, got %+v", c)
	}
	if c := colorAt(stops, 1); c != (RGB{10, 20, 30}) {
		t.Fatalf("t=1 should be the last stop, got %+v", c)
	}
	if c := colorAt(stops, 0.5); c != (RGB{5, 10, 15}) {
		t.Fatalf("midpoint should interpolate, got %+v", c)
	}
	// Three-stop: t=0.5 lands exactly on the middle stop.
	three := []RGB{{0, 0, 0}, {100, 100, 100}, {200, 200, 200}}
	if c := colorAt(three, 0.5); c != (RGB{100, 100, 100}) {
		t.Fatalf("three-stop midpoint should be the middle stop, got %+v", c)
	}
}

func TestGradientEnabledAndDisabled(t *testing.T) {
	old := Enabled
	defer func() { Enabled = old }()

	Enabled = true
	got := Gradient("abc")
	if !strings.Contains(got, "\x1b[38;2;") || !strings.HasSuffix(got, "\x1b[0m") {
		t.Fatalf("enabled gradient should emit truecolor escapes, got %q", got)
	}
	for _, r := range "abc" {
		if !strings.ContainsRune(got, r) {
			t.Fatalf("gradient dropped a rune %q: %q", r, got)
		}
	}

	Enabled = false
	if Gradient("abc") != "abc" {
		t.Fatal("disabled gradient must be plain text")
	}
}
