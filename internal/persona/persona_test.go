package persona

import (
	"os"
	"strings"
	"testing"
)

func TestPersonaResolveAndSystemNote(t *testing.T) {
	// A preset resolves to its body; SystemNote frames it as tone-only.
	body, title := Resolve("concise")
	if body == "" || title != "Concise" {
		t.Fatalf("concise resolve: got (%q, %q)", body, title)
	}
	note := SystemNote("concise")
	if !strings.Contains(note, "TONE and STYLE only") || !strings.Contains(note, "Trust Kernel") {
		t.Fatalf("SystemNote must subordinate persona to the kernel: %q", note)
	}

	// Default / unknown → no note.
	if SystemNote("default") != "" || SystemNote("") != "" || SystemNote("nope") != "" {
		t.Fatal("default/unknown personas must inject nothing")
	}
}

func TestPersonaActiveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLICHE_CONFIG_HOME", dir)

	if Active() != "" {
		t.Fatal("fresh config should have no active persona")
	}
	if err := SetActive("mentor"); err != nil {
		t.Fatalf("set mentor: %v", err)
	}
	if Active() != "mentor" {
		t.Fatalf("active = %q, want mentor", Active())
	}
	// An unknown preset is rejected (so the stored value is always resolvable).
	if err := SetActive("bogus"); err == nil {
		t.Fatal("setting an unknown persona should error")
	}
	// "custom" without a PERSONA.md is still settable but resolves to empty.
	if err := SetActive("custom"); err != nil {
		t.Fatalf("set custom: %v", err)
	}
	if b, _ := Resolve("custom"); b != "" {
		t.Fatal("custom with no PERSONA.md should resolve empty")
	}
	// Default clears it.
	if err := SetActive("default"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if Active() != "" {
		t.Fatalf("active should be cleared, got %q", Active())
	}
}

func TestPersonaCustomClipped(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLICHE_CONFIG_HOME", dir)
	big := strings.Repeat("x", maxBody+500)
	p, _ := customPath()
	if err := os.WriteFile(p, []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	if !HasCustom() {
		t.Fatal("HasCustom should be true")
	}
	body, _ := Resolve("custom")
	if len(body) > maxBody+40 {
		t.Fatalf("custom body not clipped: %d chars", len(body))
	}
}
