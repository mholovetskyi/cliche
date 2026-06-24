package cli

import (
	"bytes"
	"testing"
)

func TestApplyProviderSwitchesBackendAndResetsModel(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	t.Setenv("GROQ_API_KEY", "gsk_test") // a keyed provider → no inline prompt

	var out bytes.Buffer
	s := newMgmtSession(t, t.TempDir(), &out)
	s.cfg.Provider, s.cfg.Model = "openai", "gpt-4o-mini" // pretend we were on openai

	s.applyProvider("groq")

	if s.cfg.Provider != "groq" {
		t.Fatalf("provider = %q, want groq", s.cfg.Provider)
	}
	// Switching resets to the NEW provider's default (the old model id wouldn't be
	// valid on groq) — not the carried-over "gpt-4o-mini".
	want := builtinProviders["groq"].defaultModel
	if s.cfg.Model != want || s.a.Model() != want {
		t.Fatalf("model = %q / agent %q, want the groq default %q", s.cfg.Model, s.a.Model(), want)
	}
}

func TestApplyProviderUnknownIsRejected(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	var out bytes.Buffer
	s := newMgmtSession(t, t.TempDir(), &out)
	s.applyProvider("nonesuch")
	if got := out.String(); !contains(got, "unknown provider") {
		t.Fatalf("unknown provider should be reported, got: %q", got)
	}
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }
