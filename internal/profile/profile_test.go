package profile

import (
	"strings"
	"testing"
)

func TestProfileAppendLoadDedupe(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	if Load() != "" {
		t.Fatal("a fresh profile should be empty")
	}
	for _, f := range []string{"prefers Go, hates jank", "ships fast", "prefers Go, hates jank"} {
		if err := Append(f); err != nil {
			t.Fatal(err)
		}
	}
	got := Load()
	if !strings.Contains(got, "prefers Go") || !strings.Contains(got, "ships fast") {
		t.Fatalf("profile missing facts:\n%s", got)
	}
	if n := strings.Count(got, "prefers Go"); n != 1 {
		t.Fatalf("should dedupe, found %d", n)
	}
	if SystemNote(got) == "" {
		t.Fatal("system note should be non-empty when the profile is set")
	}
	if SystemNote("") != "" {
		t.Fatal("empty profile → empty note")
	}
	if err := Append("   "); err == nil {
		t.Fatal("empty fact should error")
	}
}
