package cli

import (
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/verifier"
)

func TestResolveBackendAutoDetect(t *testing.T) {
	base := config.Default() // provider "anthropic", model "claude-sonnet-4-6"

	t.Run("auto-detects openrouter when only its key is set", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENROUTER_API_KEY", "sk-or-x")
		t.Setenv("OPENAI_API_KEY", "")
		prov, model, err := resolveBackend(base, &runFlags{})
		if err != nil {
			t.Fatal(err)
		}
		if prov != "openrouter" {
			t.Fatalf("provider = %q, want openrouter (Cliche must not be Anthropic-by-default)", prov)
		}
		if model != "anthropic/claude-sonnet-4.6" {
			t.Fatalf("model = %q, want the openrouter default (not the Anthropic id)", model)
		}
	})

	t.Run("prefers anthropic when its key is present", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "sk-ant")
		t.Setenv("OPENROUTER_API_KEY", "sk-or")
		t.Setenv("OPENAI_API_KEY", "")
		prov, model, err := resolveBackend(base, &runFlags{})
		if err != nil || prov != "anthropic" || model != "claude-sonnet-4-6" {
			t.Fatalf("got prov=%q model=%q err=%v", prov, model, err)
		}
	})

	t.Run("explicit --provider is respected and errors without its key", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENROUTER_API_KEY", "sk-or")
		t.Setenv("OPENAI_API_KEY", "")
		if _, _, err := resolveBackend(base, &runFlags{provider: "anthropic"}); err == nil {
			t.Fatal("explicit --provider anthropic with no key must error, not silently auto-switch")
		}
	})

	t.Run("no key at all errors listing every option", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENROUTER_API_KEY", "")
		t.Setenv("OPENAI_API_KEY", "")
		_, _, err := resolveBackend(base, &runFlags{})
		if err == nil {
			t.Fatal("no keys must error")
		}
		for _, want := range []string{"ANTHROPIC_API_KEY", "OPENROUTER_API_KEY", "OPENAI_API_KEY"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error should name %s, got: %v", want, err)
			}
		}
	})

	t.Run("explicit --model overrides the provider default", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("OPENROUTER_API_KEY", "sk-or")
		t.Setenv("OPENAI_API_KEY", "")
		if _, model, _ := resolveBackend(base, &runFlags{model: "mistralai/mixtral"}); model != "mistralai/mixtral" {
			t.Fatalf("model = %q, want the explicit override", model)
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
