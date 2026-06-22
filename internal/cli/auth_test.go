package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/secrets"
)

func TestCmdAuthSaveAndStatus(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	keyFile := filepath.Join(t.TempDir(), "key.txt")
	if err := os.WriteFile(keyFile, []byte("sk-or-secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if code := cmdAuth([]string{"openrouter", "--from-file", keyFile}, &out, &out); code != 0 {
		t.Fatalf("auth save exit = %d:\n%s", code, out.String())
	}
	if k, _ := secrets.Lookup("openrouter"); k != "sk-or-secret" {
		t.Fatalf("key not persisted, Lookup got %q", k)
	}

	// Status shows openrouter configured, others not, and never prints the key.
	out.Reset()
	cmdAuth(nil, &out, &out)
	s := out.String()
	if !strings.Contains(s, "openrouter") || !strings.Contains(s, "configured") {
		t.Fatalf("status should show openrouter configured:\n%s", s)
	}
	if !strings.Contains(s, "not set") {
		t.Fatalf("status should show unconfigured providers:\n%s", s)
	}
	if strings.Contains(s, "sk-or-secret") {
		t.Fatalf("status must never print the key:\n%s", s)
	}

	// Unknown provider is a usage error.
	out.Reset()
	if code := cmdAuth([]string{"bogus", "--key", "x"}, &out, &out); code != 2 {
		t.Fatalf("unknown provider should exit 2, got %d", code)
	}

	// Removal clears the saved key.
	out.Reset()
	if code := cmdAuth([]string{"openrouter", "--remove"}, &out, &out); code != 0 {
		t.Fatalf("auth remove exit = %d:\n%s", code, out.String())
	}
	if k, _ := secrets.Lookup("openrouter"); k != "" {
		t.Fatalf("key should be removed, Lookup got %q", k)
	}
}

func TestCmdAuthRequiresKeySource(t *testing.T) {
	t.Setenv("CLICHE_CONFIG_HOME", t.TempDir())
	var out bytes.Buffer
	// No --key/--from-file and stdin is a terminal in tests → guidance, exit 2.
	if code := cmdAuth([]string{"openai"}, &out, &out); code != 2 {
		t.Fatalf("missing key source should exit 2, got %d:\n%s", code, out.String())
	}
}
