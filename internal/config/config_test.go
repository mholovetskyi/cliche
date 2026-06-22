package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadNoFileIsDefault(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("Load with no file should equal Default()")
	}
}

func TestLoadPartialKeepsDefaults(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(Dir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(Dir(root), "config.json"), []byte(`{"model":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "x" {
		t.Fatalf("model override lost: %q", cfg.Model)
	}
	if cfg.Governor.MaxTurns != Default().Governor.MaxTurns {
		t.Fatalf("absent fields should keep defaults, got MaxTurns=%d", cfg.Governor.MaxTurns)
	}
}

func TestLoadMalformedErrors(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(Dir(root), 0o755)
	_ = os.WriteFile(filepath.Join(Dir(root), "config.json"), []byte("{not json"), 0o644)
	if _, err := Load(root); err == nil {
		t.Fatal("malformed config should error")
	}
}

func TestValidate(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
	bad := []func(*Config){
		func(c *Config) { c.Budget.MaxUSD = 0; c.Budget.MaxTokens = 0 },
		func(c *Config) { c.Budget.MaxTokens = -1 },
		func(c *Config) { c.Governor.MaxTurns = 0 },
		func(c *Config) { c.Governor.RepetitionWindow = 2; c.Governor.RepetitionThreshold = 5 },
	}
	for i, mut := range bad {
		c := Default()
		mut(&c)
		if err := c.Validate(); err == nil {
			t.Fatalf("case %d should be invalid", i)
		}
	}
}

func TestHasAgentsFile(t *testing.T) {
	root := t.TempDir()
	if _, ok := HasAgentsFile(root); ok {
		t.Fatal("no agents file should report false")
	}
	_ = os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("x"), 0o644)
	name, ok := HasAgentsFile(root)
	if !ok || name != "AGENTS.md" {
		t.Fatalf("AGENTS.md should win, got %q ok=%v", name, ok)
	}
}
