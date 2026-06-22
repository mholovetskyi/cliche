package secrets

import (
	"strings"
	"testing"
)

func TestEnvVarAndKnown(t *testing.T) {
	if EnvVar("openrouter") != "OPENROUTER_API_KEY" {
		t.Fatal("openrouter env var wrong")
	}
	if EnvVar("") != "ANTHROPIC_API_KEY" {
		t.Fatal("empty provider should default to anthropic env var")
	}
	if Known("nope") || !Known("openai") {
		t.Fatal("Known() wrong")
	}
}

func TestSaveLookupRemove(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	if k, src := Lookup("openrouter"); k != "" || src != "" {
		t.Fatalf("expected nothing configured initially, got %q %q", k, src)
	}

	if _, err := Save("openrouter", "  sk-or-abc  "); err != nil {
		t.Fatal(err)
	}
	k, src := Lookup("openrouter")
	if k != "sk-or-abc" {
		t.Fatalf("key = %q, want trimmed sk-or-abc", k)
	}
	if !strings.HasPrefix(src, "file:") {
		t.Fatalf("source = %q, want file:", src)
	}

	// The environment variable must win over the saved file.
	t.Setenv("OPENROUTER_API_KEY", "env-wins")
	if k, src := Lookup("openrouter"); k != "env-wins" || !strings.HasPrefix(src, "env:") {
		t.Fatalf("env should override file, got %q %q", k, src)
	}
	t.Setenv("OPENROUTER_API_KEY", "")

	if err := Remove("openrouter"); err != nil {
		t.Fatal(err)
	}
	if k, _ := Lookup("openrouter"); k != "" {
		t.Fatalf("key should be gone after Remove, got %q", k)
	}
	// Removing an absent key is not an error.
	if err := Remove("openrouter"); err != nil {
		t.Fatalf("removing absent key should be a no-op, got %v", err)
	}
}
