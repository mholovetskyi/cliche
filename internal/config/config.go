// Package config loads run configuration from .cliche/config.json, merged
// over safe defaults. It also detects an AGENTS.md project-context file (the
// cross-tool standard Cliche adopts).
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Budget mirrors budget.Limits in plain config form.
type Budget struct {
	MaxTokens int     `json:"max_tokens"`
	MaxUSD    float64 `json:"max_usd"`
}

// Governor mirrors governor.Limits in plain config form (durations in
// seconds for human-friendly JSON).
type Governor struct {
	MaxTurns                  int `json:"max_turns"`
	MaxWallClockSeconds       int `json:"max_wallclock_seconds"`
	MaxConsecutiveFailedEdits int `json:"max_consecutive_failed_edits"`
	RepetitionWindow          int `json:"repetition_window"`
	RepetitionThreshold       int `json:"repetition_threshold"`
	NoProgressTurns           int `json:"no_progress_turns"`
}

// Verify configures the Verifier's independent test re-run.
type Verify struct {
	TestCommand string `json:"test_command"`
}

// Context configures the Context Ledger (bounded, recoverable compaction).
type Context struct {
	LimitTokens int `json:"limit_tokens"`
	KeepRecent  int `json:"keep_recent"`
}

// Config is the full run configuration.
type Config struct {
	Model    string   `json:"model"`
	Budget   Budget   `json:"budget"`
	Governor Governor `json:"governor"`
	Verify   Verify   `json:"verify"`
	Context  Context  `json:"context"`
}

// Default returns conservative, trust-first defaults.
func Default() Config {
	return Config{
		Model: "claude-sonnet-4-6",
		Budget: Budget{
			MaxTokens: 2_000_000,
			MaxUSD:    5.0,
		},
		Governor: Governor{
			MaxTurns:                  50,
			MaxWallClockSeconds:       1800,
			MaxConsecutiveFailedEdits: 5,
			RepetitionWindow:          8,
			RepetitionThreshold:       3,
			NoProgressTurns:           12,
		},
		Context: Context{
			LimitTokens: 120_000,
			KeepRecent:  12,
		},
	}
}

// Dir returns the .cliche directory under root.
func Dir(root string) string { return filepath.Join(root, ".cliche") }

// Load reads .cliche/config.json under root, merged over Default(). A missing
// file is not an error.
func Load(root string) (Config, error) {
	cfg := Default()
	path := filepath.Join(Dir(root), "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	// Unmarshal over defaults: fields present in the file win; absent fields
	// keep their default.
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// HasAgentsFile reports whether an AGENTS.md (or known fallback) exists under
// root, and which one.
func HasAgentsFile(root string) (string, bool) {
	for _, name := range []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return name, true
		}
	}
	return "", false
}
