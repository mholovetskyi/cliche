package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/config"
)

func TestConfigCommandDefault(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdConfig([]string{"--dir", t.TempDir()}, &out, &errOut)
	if code != 0 {
		t.Fatalf("default config should be valid (exit 0), got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "\"max_turns\"") || !strings.Contains(out.String(), "config is valid") {
		t.Fatalf("expected printed config + validity, got:\n%s", out.String())
	}
}

func TestConfigCommandInvalid(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(config.Dir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	// max_turns: 0 disarms the loop breaker -> invalid.
	bad := `{"governor":{"max_turns":0}}`
	if err := os.WriteFile(filepath.Join(config.Dir(dir), "config.json"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := cmdConfig([]string{"--dir", dir}, &out, &errOut); code != 2 {
		t.Fatalf("invalid config should exit 2, got %d", code)
	}
	if !strings.Contains(errOut.String(), "INVALID") {
		t.Fatalf("expected an INVALID message, got: %s", errOut.String())
	}
}
