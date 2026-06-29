package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/cron"
	"github.com/mholovetskyi/cliche/internal/secrets"
	"github.com/mholovetskyi/cliche/internal/style"
)

// dcheck is one preflight result. status is "ok" | "warn" | "fail".
type dcheck struct {
	status string
	label  string
	hint   string
}

// cmdDoctor is a one-shot environment + health preflight: providers, the
// toolchain, the Trust Kernel, and the optional integrations — so a new user can
// see at a glance what's set up and what to fix. Exit 1 if a critical thing
// (no provider key, invalid config) is missing.
func cmdDoctor(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project root")
	_ = fs.Parse(args)

	fmt.Fprintln(out, "\n  "+style.BoldWhite("Cliché doctor")+style.Gray("  ·  environment + health check"))
	fails := 0
	for _, c := range doctorChecks(*dir) {
		mark := style.Green("✓")
		switch c.status {
		case "warn":
			mark = style.Gray("⚠")
		case "fail":
			mark = style.Red("✗")
			fails++
		}
		fmt.Fprintln(out, "  "+mark+" "+c.label)
		if c.hint != "" && c.status != "ok" {
			fmt.Fprintln(out, "      "+style.Gray("→ "+c.hint))
		}
	}
	fmt.Fprintln(out)
	if fails == 0 {
		fmt.Fprintln(out, "  "+style.Green("all good — you're ready to build."))
		return 0
	}
	fmt.Fprintln(out, "  "+style.Red(fmt.Sprintf("%d issue(s) need attention.", fails)))
	return 1
}

func doctorChecks(root string) []dcheck {
	var cs []dcheck
	cfg, cfgErr := config.Load(root)

	// Providers — at least one key is required to run anything.
	var configured []string
	for _, p := range allProviderNames(cfg) {
		if k, _ := secrets.Lookup(p); k != "" {
			configured = append(configured, p)
		}
	}
	if len(configured) > 0 {
		cs = append(cs, dcheck{"ok", "provider key configured: " + strings.Join(configured, ", "), ""})
	} else {
		cs = append(cs, dcheck{"fail", "no provider API key", "run `cliche login` (or export " + secrets.EnvVar("anthropic") + ")"})
	}

	// Toolchain — the agent shells out to these; missing ones limit what it can do.
	for _, t := range []struct{ bin, why string }{
		{"git", "version control, the Git tab, Deploy"},
		{"gh", "open PRs + one-click Deploy"},
		{"node", "building web apps (npm / vite)"},
		{"go", "building Go projects"},
	} {
		if p, err := exec.LookPath(t.bin); err == nil {
			cs = append(cs, dcheck{"ok", t.bin + "  " + style.Gray("("+p+")"), ""})
		} else {
			cs = append(cs, dcheck{"warn", t.bin + " not found — " + t.why, "install " + t.bin})
		}
	}
	if b := doctorBrowser(); b != "" {
		cs = append(cs, dcheck{"ok", "browser for screenshots: " + style.Gray(b), ""})
	} else {
		cs = append(cs, dcheck{"warn", "no Chrome/Edge/Chromium — the screenshot tool is off", "install a Chromium browser or set CLICHE_BROWSER"})
	}

	// Trust Kernel.
	if cfgErr != nil {
		cs = append(cs, dcheck{"fail", "config invalid: " + cfgErr.Error(), "fix .cliche/config.json"})
	} else {
		cs = append(cs, dcheck{"ok", fmt.Sprintf("config valid · budget cap $%.2f · governor %d turns", cfg.Budget.MaxUSD, cfg.Governor.MaxTurns), ""})
	}
	if _, err := secrets.SigningKey(); err == nil {
		cs = append(cs, dcheck{"ok", "ledger signing key present (audit seal)", ""})
	} else {
		cs = append(cs, dcheck{"warn", "no ledger signing key — seals unavailable", ""})
	}

	// Optional integrations.
	if os.Getenv("CLICHE_TELEGRAM_TOKEN") != "" {
		cs = append(cs, dcheck{"ok", "Telegram bot token set", ""})
	} else {
		cs = append(cs, dcheck{"warn", "Telegram bot off (no token)", "set CLICHE_TELEGRAM_TOKEN to use `cliche telegram`"})
	}
	jobs, _ := cron.Load(root)
	cs = append(cs, dcheck{"ok", fmt.Sprintf("%d scheduled cron job(s)", len(jobs)), ""})

	return cs
}

// doctorBrowser mirrors the screenshot tool's discovery enough to report it.
func doctorBrowser() string {
	if b := os.Getenv("CLICHE_BROWSER"); b != "" {
		if p, err := exec.LookPath(b); err == nil {
			return p
		}
		if _, err := os.Stat(b); err == nil {
			return b
		}
	}
	for _, n := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "chrome", "msedge"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	for _, p := range []string{
		os.Getenv("ProgramFiles(x86)") + `\Microsoft\Edge\Application\msedge.exe`,
		os.Getenv("ProgramFiles") + `\Google\Chrome\Application\chrome.exe`,
	} {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
