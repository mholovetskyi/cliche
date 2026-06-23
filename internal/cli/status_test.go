package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/budget"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/governor"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/style"
	"github.com/mholovetskyi/cliche/internal/tools"
)

func TestShowStatusAndRules(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = false, true // plain mode: assert on the text
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	led, _ := ledger.Open(t.TempDir())
	a := agent.New(
		provider.NewMock("mock", provider.NormalScript(), false),
		budget.New(budget.Limits{MaxTokens: 1_000_000, MaxUSD: 100}),
		governor.DefaultLimits(),
		led, tools.SimExecutor{}, agent.Config{Model: "openai/gpt-4o-mini"},
	)
	cfg := config.Config{
		Provider:    "openrouter",
		Governor:    config.Governor{MaxTurns: 50, MaxWallClockSeconds: 300, MaxConsecutiveFailedEdits: 3},
		Context:     config.Context{LimitTokens: 120_000},
		Permissions: config.Permissions{Allow: []string{"Read(**)"}, Deny: []string{"Bash(rm *)"}},
		Egress:      config.Egress{Allow: []string{"api.github.com"}},
		Hooks:       config.Hooks{PreToolUse: "./hook.sh"},
	}
	app := &approver{mode: modeAutoEdit}
	app.alwaysRun = true // a standing "always allow commands" grant

	var out bytes.Buffer
	s := &session{a: a, out: &out, cfg: cfg, app: app}

	s.showStatus()
	st := out.String()
	for _, want := range []string{"status", "auto-edit", "gpt-4o-mini", "budget", "context", "governor", "50 turns", "always: commands"} {
		if !strings.Contains(st, want) {
			t.Errorf("/status missing %q:\n%s", want, st)
		}
	}

	out.Reset()
	s.showRules()
	rl := out.String()
	for _, want := range []string{"rules in force", "Read(**)", "Bash(rm *)", "api.github.com", "./hook.sh"} {
		if !strings.Contains(rl, want) {
			t.Errorf("/rules missing %q:\n%s", want, rl)
		}
	}

	// With no policy configured, /rules states the safe defaults rather than blanks.
	out.Reset()
	(&session{a: a, out: &out, cfg: config.Config{Provider: "openrouter"}, app: &approver{}}).showRules()
	for _, want := range []string{"no hard denies", "unrestricted", "none"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("/rules empty-state missing %q:\n%s", want, out.String())
		}
	}
}
