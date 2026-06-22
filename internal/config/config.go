// Package config loads run configuration from .cliche/config.json, merged
// over safe defaults. It also detects an AGENTS.md project-context file (the
// cross-tool standard Cliche adopts).
package config

import (
	"encoding/json"
	"fmt"
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

// Subagents configures subagent delegation.
type Subagents struct {
	MaxDepth int `json:"max_depth"` // 0 disables subagents
}

// Permissions holds fine-grained allow/deny rules (deterministic policy-as-code).
// Rule syntax: Tool(pattern) — Read/Write/Edit(path glob, ** spans dirs) or
// Bash(command glob, * = any). Deny wins over allow and overrides --yolo.
type Permissions struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// Egress is the network host allowlist for agent-initiated fetches (web_fetch).
// Empty means unrestricted (the --allow-web gate still applies); when set, only
// matching hosts are reachable. Patterns: "api.github.com", "*.github.com", "*".
type Egress struct {
	Allow []string `json:"allow,omitempty"`
}

// ProviderDef defines (or overrides) a model provider, so Cliche can connect to
// literally any OpenAI-compatible API — a hosted service or a local server
// (Ollama, LM Studio, vLLM). The key is read from <NAME>_API_KEY in the
// environment, or saved with `cliche auth <name>`.
type ProviderDef struct {
	Name         string `json:"name"`
	BaseURL      string `json:"base_url"`      // OpenAI-compatible chat-completions endpoint
	DefaultModel string `json:"default_model"` // used when --model is not given
}

// MCPServer configures one Model Context Protocol server. A server is reached
// over stdio (a launched Command) or, when URL is set, over Streamable HTTP.
type MCPServer struct {
	Name    string   `json:"name"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
	URL     string   `json:"url,omitempty"` // Streamable-HTTP endpoint (overrides Command)
}

// Config is the full run configuration.
type Config struct {
	Model       string        `json:"model"`
	Provider    string        `json:"provider"` // anthropic | openrouter | openai
	BaseURL     string        `json:"base_url"` // override the provider's API endpoint
	Budget      Budget        `json:"budget"`
	Governor    Governor      `json:"governor"`
	Verify      Verify        `json:"verify"`
	Context     Context       `json:"context"`
	Subagents   Subagents     `json:"subagents"`
	MCP         []MCPServer   `json:"mcp,omitempty"`
	Providers   []ProviderDef `json:"providers,omitempty"`
	Permissions Permissions   `json:"permissions,omitempty"`
	Egress      Egress        `json:"egress,omitempty"`
}

// Default returns conservative, trust-first defaults.
func Default() Config {
	return Config{
		Model:    "claude-sonnet-4-6",
		Provider: "anthropic",
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
		Subagents: Subagents{
			MaxDepth: 2,
		},
	}
}

// Validate rejects configurations that would silently disarm a guardrail — the
// worst-case failure for a trust tool. It fails loudly, naming the field.
func (c Config) Validate() error {
	b, g := c.Budget, c.Governor
	switch {
	case b.MaxTokens < 0:
		return fmt.Errorf("budget.max_tokens must be >= 0 (got %d)", b.MaxTokens)
	case b.MaxUSD < 0:
		return fmt.Errorf("budget.max_usd must be >= 0 (got %v)", b.MaxUSD)
	case b.MaxTokens == 0 && b.MaxUSD == 0:
		return fmt.Errorf("at least one of budget.max_tokens or budget.max_usd must be set (both 0 disarms the spend cap)")
	case g.MaxTurns <= 0:
		return fmt.Errorf("governor.max_turns must be > 0 (got %d; 0 disarms the loop breaker)", g.MaxTurns)
	case g.MaxWallClockSeconds < 0:
		return fmt.Errorf("governor.max_wallclock_seconds must be >= 0 (got %d)", g.MaxWallClockSeconds)
	case g.MaxConsecutiveFailedEdits < 0 || g.RepetitionWindow < 0 || g.RepetitionThreshold < 0 || g.NoProgressTurns < 0:
		return fmt.Errorf("governor limits must be >= 0")
	case g.RepetitionThreshold > 0 && g.RepetitionWindow > 0 && g.RepetitionWindow < g.RepetitionThreshold:
		return fmt.Errorf("governor.repetition_window (%d) must be >= repetition_threshold (%d) or the breaker never trips", g.RepetitionWindow, g.RepetitionThreshold)
	case c.Subagents.MaxDepth < 0:
		return fmt.Errorf("subagents.max_depth must be >= 0 (got %d)", c.Subagents.MaxDepth)
	}
	for i, s := range c.MCP {
		if s.Name == "" {
			return fmt.Errorf("mcp[%d]: name is required", i)
		}
		if s.Command == "" && s.URL == "" {
			return fmt.Errorf("mcp[%d] (%s): set either command (stdio) or url (HTTP)", i, s.Name)
		}
	}
	for i, p := range c.Providers {
		if p.Name == "" || p.BaseURL == "" {
			return fmt.Errorf("providers[%d]: both name and base_url are required", i)
		}
	}
	return nil
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
