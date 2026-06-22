package secrets

import (
	"strings"
	"testing"
)

func TestEnvVar(t *testing.T) {
	cases := map[string]string{
		"openrouter": "OPENROUTER_API_KEY",
		"":           "ANTHROPIC_API_KEY", // empty → anthropic
		"groq":       "GROQ_API_KEY",
		"deepseek":   "DEEPSEEK_API_KEY",
		"my-local":   "MY_LOCAL_API_KEY", // dashes become underscores
	}
	for in, want := range cases {
		if got := EnvVar(in); got != want {
			t.Errorf("EnvVar(%q) = %q, want %q", in, got, want)
		}
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
