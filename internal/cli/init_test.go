package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/config"
)

func TestInitScaffoldsAndIsValid(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	if code := cmdInit([]string{"--dir", root}, &out, &out); code != 0 {
		t.Fatalf("init exit code = %d, want 0:\n%s", code, out.String())
	}

	// config.json exists, loads, and passes validation (no disarmed guardrail).
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("scaffolded config should load: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("scaffolded config should be valid: %v", err)
	}
	if !reflect.DeepEqual(cfg, config.Default()) {
		t.Fatalf("scaffolded config should equal defaults")
	}

	// AGENTS.md scaffolded with a verify section.
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md should be created: %v", err)
	}
	if !strings.Contains(string(agents), "## verify") {
		t.Fatal("AGENTS.md template should include a verify section")
	}
}

func TestInitIsIdempotentAndNonDestructive(t *testing.T) {
	root := t.TempDir()
	// Pre-existing files must be preserved verbatim.
	if err := os.MkdirAll(config.Dir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(config.Dir(root), "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"model":"my-model"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if code := cmdInit([]string{"--dir", root}, &out, &out); code != 0 {
		t.Fatalf("init exit code = %d, want 0", code)
	}
	if got, _ := os.ReadFile(cfgPath); string(got) != `{"model":"my-model"}` {
		t.Fatalf("init must not overwrite an existing config, got %q", got)
	}
	// An existing project-context file means no AGENTS.md is written.
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatal("init should not create AGENTS.md when CLAUDE.md already exists")
	}
	if !strings.Contains(out.String(), "kept existing") {
		t.Fatalf("init should report kept files:\n%s", out.String())
	}
}
