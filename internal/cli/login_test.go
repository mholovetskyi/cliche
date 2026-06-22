package cli

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/secrets"
)

// stubValidator swaps the network key-check for the duration of a test.
func stubValidator(t *testing.T, fn func(name, key string) error) {
	t.Helper()
	orig := validateKey
	validateKey = func(_ context.Context, name, key, _ string) error { return fn(name, key) }
	t.Cleanup(func() { validateKey = orig })
}

func loginEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
}

func TestRunLoginSavesValidatedKey(t *testing.T) {
	loginEnv(t)
	stubValidator(t, func(name, key string) error {
		if name == "openrouter" && key == "sk-good" {
			return nil
		}
		return provider.ErrUnauthorized
	})

	in := bufio.NewReader(strings.NewReader("2\nsk-good\n")) // 2 = OpenRouter
	var out bytes.Buffer
	if code := runLogin(in, &out); code != 0 {
		t.Fatalf("login exit = %d:\n%s", code, out.String())
	}
	if k, _ := secrets.Lookup("openrouter"); k != "sk-good" {
		t.Fatalf("key not saved, got %q", k)
	}
	if !strings.Contains(out.String(), "you're set") {
		t.Fatalf("expected success message:\n%s", out.String())
	}
}

func TestRunLoginRetriesAfterRejectedKey(t *testing.T) {
	loginEnv(t)
	stubValidator(t, func(name, key string) error {
		if key == "sk-good" {
			return nil
		}
		return provider.ErrUnauthorized
	})

	in := bufio.NewReader(strings.NewReader("2\nsk-bad\nsk-good\n"))
	var out bytes.Buffer
	if code := runLogin(in, &out); code != 0 {
		t.Fatalf("login should recover after a rejected key, exit = %d:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "rejected") {
		t.Fatalf("expected a rejection notice before the retry:\n%s", out.String())
	}
	if k, _ := secrets.Lookup("openrouter"); k != "sk-good" {
		t.Fatalf("the good key should be saved, got %q", k)
	}
}

func TestRunLoginReprompptsOnBadChoice(t *testing.T) {
	loginEnv(t)
	stubValidator(t, func(_, _ string) error { return nil })

	in := bufio.NewReader(strings.NewReader("nope\n1\nsk-x\n")) // bad, then 1 = Anthropic
	var out bytes.Buffer
	if code := runLogin(in, &out); code != 0 {
		t.Fatalf("login exit = %d:\n%s", code, out.String())
	}
	if k, _ := secrets.Lookup("anthropic"); k != "sk-x" {
		t.Fatalf("expected anthropic key saved after reprompt, got %q", k)
	}
}
