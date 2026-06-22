package cli

import (
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/verifier"
)

func TestResolveBackendAutoDetect(t *testing.T) {
	// Isolate from the real per-user credentials file so saved keys don't leak in.
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	base := config.Default() // provider "anthropic", model "claude-sonnet-4-6"

	t.Run("auto-detects openrouter when only its key is set", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENROUTER_API_KEY", "sk-or-x")
		t.Setenv("OPENAI_API_KEY", "")
		b, err := resolveBackend(base, &runFlags{})
		if err != nil {
			t.Fatal(err)
		}
		if b.name != "openrouter" {
			t.Fatalf("provider = %q, want openrouter (Cliche must not be Anthropic-by-default)", b.name)
		}
		if b.model != "openai/gpt-4o-mini" {
			t.Fatalf("model = %q, want the openrouter default (not the Anthropic id)", b.model)
		}
		if b.native || b.baseURL == "" {
			t.Fatalf("openrouter should be OpenAI-compatible with a base URL, got native=%v base=%q", b.native, b.baseURL)
		}
	})

	t.Run("prefers anthropic when its key is present", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "sk-ant")
		t.Setenv("OPENROUTER_API_KEY", "sk-or")
		t.Setenv("OPENAI_API_KEY", "")
		b, err := resolveBackend(base, &runFlags{})
		if err != nil || b.name != "anthropic" || b.model != "claude-sonnet-4-6" || !b.native {
			t.Fatalf("got %+v err=%v", b, err)
		}
	})

	t.Run("auto-detects a third-party provider (groq)", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENROUTER_API_KEY", "")
		t.Setenv("OPENAI_API_KEY", "")
		t.Setenv("GROQ_API_KEY", "gsk-x")
		b, err := resolveBackend(base, &runFlags{})
		if err != nil || b.name != "groq" || b.baseURL == "" {
			t.Fatalf("expected groq auto-detected with a base URL, got %+v err=%v", b, err)
		}
	})

	t.Run("explicit --provider is respected and errors without its key", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENROUTER_API_KEY", "sk-or")
		t.Setenv("OPENAI_API_KEY", "")
		if _, err := resolveBackend(base, &runFlags{provider: "anthropic"}); err == nil {
			t.Fatal("explicit --provider anthropic with no key must error, not silently auto-switch")
		}
	})

	t.Run("custom provider via --base-url", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENROUTER_API_KEY", "")
		t.Setenv("OPENAI_API_KEY", "")
		t.Setenv("LOCAL_API_KEY", "x")
		b, err := resolveBackend(base, &runFlags{provider: "local", baseURL: "http://localhost:11434/v1/chat/completions", model: "llama3"})
		if err != nil || b.baseURL != "http://localhost:11434/v1/chat/completions" || b.model != "llama3" || b.native {
			t.Fatalf("custom provider via --base-url failed: %+v err=%v", b, err)
		}
	})

	t.Run("no key at all errors with a setup hint", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENROUTER_API_KEY", "")
		t.Setenv("OPENAI_API_KEY", "")
		t.Setenv("GROQ_API_KEY", "")
		_, err := resolveBackend(base, &runFlags{})
		if err == nil || !strings.Contains(err.Error(), "cliche login") {
			t.Fatalf("no keys should error pointing at cliche login, got: %v", err)
		}
	})

	t.Run("explicit --model overrides the provider default", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENROUTER_API_KEY", "sk-or")
		t.Setenv("OPENAI_API_KEY", "")
		if b, _ := resolveBackend(base, &runFlags{model: "mistralai/mixtral"}); b.model != "mistralai/mixtral" {
			t.Fatalf("model = %q, want the explicit override", b.model)
		}
	})
}

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		name string
		o    agent.Outcome
		want int
	}{
		{"completed", agent.Outcome{Stop: agent.StopCompleted}, 0},
		{"budget", agent.Outcome{Stop: agent.StopBudget}, 3},
		{"governor", agent.Outcome{Stop: "repetition"}, 4},
		{"completed+flagged", agent.Outcome{Stop: agent.StopCompleted, Verdict: verifier.StatusFlagged}, 5},
		// A budget/governor stop must win over a verdict (precedence regression).
		{"budget+flagged", agent.Outcome{Stop: agent.StopBudget, Verdict: verifier.StatusFlagged}, 3},
		{"completed+verified", agent.Outcome{Stop: agent.StopCompleted, Verdict: verifier.StatusVerified}, 0},
	}
	for _, c := range cases {
		if got := exitCodeFor(c.o); got != c.want {
			t.Errorf("%s: exitCodeFor = %d, want %d", c.name, got, c.want)
		}
	}
}
