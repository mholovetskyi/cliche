// Package cli is the command dispatcher for the cliche binary. It is built on
// the Go standard library only — zero third-party dependencies — which is
// itself on-brand: a single static binary with no supply-chain surface.
package cli

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/mholovetskyi/cliche/internal/budget"
	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/governor"
	"github.com/mholovetskyi/cliche/internal/style"
)

// Version is the build version, overridable via -ldflags.
var Version = "0.0.1-dev"

// versionString prefers an explicit ldflags Version, then the module version
// embedded by `go install`, then the dev default.
func versionString() string {
	if Version != "0.0.1-dev" {
		return "cliche " + Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return "cliche " + bi.Main.Version
	}
	return "cliche " + Version
}

// Main is the entrypoint. It returns a process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	if t := os.Getenv("CLICHE_THEME"); t != "" {
		style.ApplyTheme(t) // env theme applies globally, before any rendering
	}
	if len(args) < 2 {
		fmt.Fprint(stdout, splash())
		fmt.Fprintln(stdout, "  "+style.Gray("run `cliche help` for every command and flag."))
		return 0
	}
	cmd, rest := args[1], args[2:]
	switch cmd {
	case "help", "-h", "--help":
		usage(stdout)
		return 0
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, versionString())
		return 0
	case "init":
		return cmdInit(rest, stdout, stderr)
	case "auth":
		return cmdAuth(rest, stdout, stderr)
	case "login":
		return cmdLogin(rest, stdout, stderr)
	case "chat":
		return cmdChat(rest, stdout, stderr)
	case "sessions":
		return cmdSessions(rest, stdout, stderr)
	case "demo":
		return cmdDemo(stdout)
	case "cost":
		return cmdCost(rest, stdout, stderr)
	case "report":
		return cmdReport(rest, stdout, stderr)
	case "models":
		return cmdModels(rest, stdout, stderr)
	case "commands":
		return cmdCommands(rest, stdout, stderr)
	case "skills":
		return cmdSkills(rest, stdout, stderr)
	case "plugins":
		return cmdPlugins(rest, stdout, stderr)
	case "themes":
		return cmdThemes(rest, stdout, stderr)
	case "mcp":
		return cmdMcp(rest, stdout, stderr)
	case "bug":
		return cmdBug(rest, stdout, stderr)
	case "insights":
		return cmdInsights(rest, stdout, stderr)
	case "config":
		return cmdConfig(rest, stdout, stderr)
	case "verify":
		return cmdVerify(rest, stdout, stderr)
	case "map":
		return cmdMap(rest, stdout, stderr)
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
	fmt.Fprintln(w, "\n  "+gradientWordmark()+style.Gray("  the AI coding agent you can actually leave running"))
	fmt.Fprintln(w, "  "+style.GradientRule(58))
	fmt.Fprint(w, `
Hard spend caps. A loop circuit-breaker. A verifier that catches the agent
faking it. On by default, open, and auditable.

USAGE:
  cliche <command> [flags]

COMMANDS:
  init               Scaffold .cliche/config.json and an AGENTS.md template
                     (never overwrites existing files).
  login              Interactive setup: pick a provider, paste your key (hidden),
                     Cliche verifies it works and saves it. Run once.
  auth               Save a provider API key non-interactively (scripts/CI; no
                     provider = show status): cliche auth openrouter --from-file key.txt
  chat               Start an interactive agentic session: type a prompt and it
                     cooks (reads/edits files, runs commands), then ask again.
                     Live activity, ask-before-acting, session-wide budget.
                     Resume with --continue or --resume <id>.
  sessions           List saved chat sessions (resume one with chat --resume).
  run "<prompt>"     One-shot agent run on a prompt (BYO key, multi-turn tools).
  exec               Headless mode: prompt via -p or stdin, JSON output, clean
                     exit codes. Fails loudly on caps and breakers.
  verify             Independently re-run the project's tests and combine with
                     reward-hack detectors into a verdict (verified/flagged).
  map                Print the project repo map (the structural overview the
                     agent starts with): --dir <path>, --full to skip the bound.
  demo               Run the Trust Kernel offline against four scenarios
                     (healthy task, runaway loop, budget blowout, reward-hack).
  cost               Summarize the cost ledger for this project.
  report             Export the ledger as a Markdown verdict (task, cost,
                     changes, verdict): --out <file>, or --pr <n> to post to a
                     GitHub PR via gh.
  models             Show the maintained price table behind dollar estimates.
  commands           Custom slash commands (saved prompts): list, or
                     'commands new <name>' → .cliche/commands/<name>.md (run /<name>).
  skills             Skills the agent uses automatically: list, or
                     'skills new <name>' → .cliche/skills/<name>/SKILL.md.
  plugins            Installable bundles (skills + commands + hooks + MCP): list,
                     or 'plugins new <name>' → .cliche/plugins/<name>/.
  themes             List UI palettes (set via CLICHE_THEME or "theme" in config).
  insights           Usage & spend report from the ledger and saved sessions.
  bug                Write a bug report (environment + context) + a GitHub link.
  config             Print and validate the effective configuration.
  version            Print the version.
  help               Show this help.

CHAT/RUN/EXEC FLAGS:
  --model <id>        Model id (default from config or claude-sonnet-4-6).
  --max-usd <n>       Estimated dollar cap (hard token cap is also enforced).
  --max-tokens <n>    Hard token cap.
  --max-turns <n>     Governor turn limit (run/exec).
  --allow-write       Permit file writes without asking.
  --allow-run         Permit shell commands without asking.
  --allow-web         Permit web_fetch network access without asking.
  --sandbox           Strict posture: confine to root (overrides
                      --allow-outside-root), deny network unless an egress
                      allowlist is set, and scrub provider keys from commands.
  --mode <m>          Permission mode: plan (read-only) | suggest (ask) |
                      auto-edit (auto edits, ask commands) | full (auto all).
  --yolo              Skip approvals — but NEVER the budget cap or the governor.
  --verify            After completion, re-run tests and report a verdict.
  --allow-outside-root  Permit file access outside the project root (off by default).
  --dir <path>        Project root (default "."); file tools are confined to it.

In an interactive 'chat' (a TTY), writes/commands prompt y/N/always unless a
flag pre-authorizes them. Slash commands: /cost /clear /verify /help /exit.

EXAMPLES:
  cliche chat
  cliche demo
  cliche run --max-usd 0.50 --allow-write --verify "fix the failing test in ./api"
  git diff | cliche exec -p "review this change" --max-usd 0.10
  git diff | cliche verify --claim-pass
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

func buildGovernorLimits(cfg config.Config, maxTurns int) governor.Limits {
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
	return lim
}
