// Package cli is the command dispatcher for the cliche binary. It is built on
// the Go standard library only — zero third-party dependencies — which is
// itself on-brand: a single static binary with no supply-chain surface.
package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/mholovetskyi/cliche/internal/budget"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/governor"
)

// Version is the build version, overridable via -ldflags.
var Version = "0.0.1-dev"

// Main is the entrypoint. It returns a process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		usage(stdout)
		return 0
	}
	cmd, rest := args[1], args[2:]
	switch cmd {
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "cliche %s\n", Version)
		return 0
	case "demo":
		return cmdDemo(stdout)
	case "cost":
		return cmdCost(rest, stdout, stderr)
	case "run":
		return cmdRun(rest, stdout, stderr)
	case "exec":
		return cmdExec(rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "cliche: unknown command %q\n\n", cmd)
		usage(stderr)
		return 2
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `cliche — the AI coding agent you can actually leave running.

Hard spend caps. A loop circuit-breaker. A verifier that catches the agent
faking it. On by default, open, and auditable.

USAGE:
  cliche <command> [flags]

COMMANDS:
  run "<prompt>"     Run the agent on a prompt (BYO key via ANTHROPIC_API_KEY).
  exec               Headless mode: prompt via -p or stdin, JSON output, clean
                     exit codes. Fails loudly on caps and breakers.
  demo               Run the Trust Kernel offline against four scenarios
                     (healthy task, runaway loop, budget blowout, reward-hack).
  cost               Summarize the cost ledger for this project.
  version            Print the version.
  help               Show this help.

RUN/EXEC FLAGS:
  --model <id>        Model id (default from config or claude-sonnet-4-6).
  --max-usd <n>       Estimated dollar cap (hard token cap is also enforced).
  --max-tokens <n>    Hard token cap.
  --max-turns <n>     Governor turn limit.
  --allow-write       Permit file writes (off by default).
  --allow-run         Permit shell commands (off by default).
  --yolo              Skip approvals — but NEVER the budget cap or the governor.
  --dir <path>        Project root (default ".").

EXAMPLES:
  cliche demo
  cliche run --max-usd 0.50 "fix the failing test in ./api"
  git diff | cliche exec -p "review this change" --max-usd 0.10
`)
}

// buildLimits merges config defaults with optional flag overrides (a negative
// value means "unset / use config").
func buildBudget(cfg config.Config, maxUSD float64, maxTokens int) *budget.Kernel {
	lim := budget.Limits{MaxTokens: cfg.Budget.MaxTokens, MaxUSD: cfg.Budget.MaxUSD}
	if maxTokens >= 0 {
		lim.MaxTokens = maxTokens
	}
	if maxUSD >= 0 {
		lim.MaxUSD = maxUSD
	}
	return budget.New(lim)
}

func buildGovernor(cfg config.Config, maxTurns int) *governor.Governor {
	g := cfg.Governor
	lim := governor.Limits{
		MaxTurns:                  g.MaxTurns,
		MaxWallClock:              time.Duration(g.MaxWallClockSeconds) * time.Second,
		MaxConsecutiveFailedEdits: g.MaxConsecutiveFailedEdits,
		RepetitionWindow:          g.RepetitionWindow,
		RepetitionThreshold:       g.RepetitionThreshold,
		NoProgressTurns:           g.NoProgressTurns,
	}
	if maxTurns >= 0 {
		lim.MaxTurns = maxTurns
	}
	return governor.New(lim)
}
