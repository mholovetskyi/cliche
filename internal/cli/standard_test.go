package cli

import (
	"strings"
	"testing"
)

func TestWeakModel(t *testing.T) {
	weak := []string{"gpt-4o-mini", "claude-haiku-4-5", "anthropic/claude-3.5-haiku", "google/gemini-2.0-flash-001", "llama-3-8b", ""}
	for _, m := range weak {
		if !weakModel(m) {
			t.Errorf("weakModel(%q) = false, want true", m)
		}
	}
	strong := []string{"claude-opus-4-8", "claude-sonnet-4-6", "anthropic/claude-sonnet-4.6", "gpt-5", "gemini-2.5-pro"}
	for _, m := range strong {
		if weakModel(m) {
			t.Errorf("weakModel(%q) = true, want false", m)
		}
	}
}

func TestQualityModel(t *testing.T) {
	// A weak model on a known provider gets upgraded to that provider's strong default.
	if got, bumped := qualityModel("openrouter", "openai/gpt-4o-mini"); !bumped || got != "anthropic/claude-sonnet-4.6" {
		t.Fatalf("openrouter weak → %q (bumped=%v), want anthropic/claude-sonnet-4.6", got, bumped)
	}
	if got, bumped := qualityModel("anthropic", "claude-haiku-4-5"); !bumped || got != "claude-sonnet-4-6" {
		t.Fatalf("anthropic weak → %q (bumped=%v), want claude-sonnet-4-6", got, bumped)
	}
	// A capable model is respected (no bump).
	if got, bumped := qualityModel("anthropic", "claude-opus-4-8"); bumped || got != "claude-opus-4-8" {
		t.Fatalf("a strong model must not be downgraded/changed: %q bumped=%v", got, bumped)
	}
	// Unknown / local provider is left untouched.
	if got, bumped := qualityModel("ollama", "llama3.2"); bumped || got != "llama3.2" {
		t.Fatalf("local provider must be left alone: %q bumped=%v", got, bumped)
	}
}

func TestProStandardGating(t *testing.T) {
	if proStandard(false) != "" {
		t.Fatal("proStandard(false) must be empty")
	}
	s := proStandard(true)
	if !strings.Contains(s, "PRODUCT BUILD MODE") || !strings.Contains(s, "QUALITY GATE") {
		t.Fatalf("proStandard(true) missing the bar: %q", s)
	}
}
