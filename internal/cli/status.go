package cli

import (
	"fmt"
	"strings"

	"github.com/mholovetskyi/cliche/internal/style"
)

// modeDesc is the one-line plain-English meaning of a permission mode, shared by
// /status and /rules so the wording can't drift.
func modeDesc(mode string) string {
	switch mode {
	case modePlan:
		return "read-only — no edits or commands"
	case modeAutoEdit:
		return "auto edits · asks before commands"
	case modeFull:
		return "auto everything (still budget/governor-bound)"
	default: // suggest
		return "asks before acting"
	}
}

// grantSummary describes the standing "always allow" grants — as consequential
// as the mode, but otherwise invisible between prompts.
func grantSummary(a *approver) string {
	if a == nil {
		return style.Gray("asks every time")
	}
	w, r, web := a.AlwaysFlags()
	var on []string
	if w {
		on = append(on, "edits")
	}
	if r {
		on = append(on, "commands")
	}
	if web {
		on = append(on, "fetches")
	}
	if len(on) == 0 {
		return style.Gray("asks every time")
	}
	return style.White("always: " + strings.Join(on, ", "))
}

// showStatus renders the whole trust-kernel state in one framed panel: the
// permission mode, the model, spend and context headroom (with gauges), the
// governor's loop-breaker caps, and any standing "always allow" grants. Every
// number here is already tracked by the session — /status just makes the
// product's differentiator glanceable in one place instead of three commands.
func (s *session) showStatus() {
	u := s.a.Usage()
	lim := s.a.Limits()
	const labelW = 8
	row := func(label, value string) string {
		return style.TableRow([]string{style.Gray(label), value}, []int{labelW, 0}, nil)
	}

	rows := []string{
		row("mode", style.White(s.modeName())+style.Gray(" — "+modeDesc(s.modeName()))),
		row("model", style.White(shortModel(s.a.Model()))+style.Dim(" · "+s.cfg.Provider)),
	}

	if lim.MaxUSD > 0 {
		frac := u.USD / lim.MaxUSD
		rows = append(rows, row("budget", gaugePrefix(frac, 8)+style.Gray(fmt.Sprintf("%d%% · $%.4f of $%.2f cap", pctFloat(frac), u.USD, lim.MaxUSD))))
	} else {
		rows = append(rows, row("budget", style.Gray(fmt.Sprintf("$%.4f · %s tokens (no cap)", u.USD, humanTokens(u.TotalTokens())))))
	}

	est, compactions := s.a.ContextStats()
	if l := s.cfg.Context.LimitTokens; l > 0 {
		frac := float64(est) / float64(l)
		rows = append(rows, row("context", gaugePrefix(frac, 8)+style.Gray(fmt.Sprintf("%d%% · ~%s of %s · %d compaction(s)", pctOf(est, l), humanTokens(est), humanTokens(l), compactions))))
	} else {
		rows = append(rows, row("context", style.Gray(fmt.Sprintf("~%s tokens · %d compaction(s)", humanTokens(est), compactions))))
	}

	g := s.cfg.Governor
	rows = append(rows, row("guards", style.Gray(fmt.Sprintf("governor · %d turns · %ds wall · %d failed-edits", g.MaxTurns, g.MaxWallClockSeconds, g.MaxConsecutiveFailedEdits))))
	rows = append(rows, row("allow", grantSummary(s.app)))

	fmt.Fprintln(s.out, style.Indent(style.Box("status", strings.Join(rows, "\n"), style.GrayRGB)))
}

// showRules surfaces the policy actually in force this session — allow/deny
// rules, the egress host allowlist, and lifecycle hooks — so "why was that
// blocked?" is answerable from inside the session, without opening config.json.
func (s *session) showRules() {
	const labelW = 9
	line := func(label, value string) {
		fmt.Fprintf(s.out, "  %s %s\n", style.Gray(style.Pad(label, labelW)), value)
	}
	join := func(items []string, empty string) string {
		if len(items) == 0 {
			return style.Gray(empty)
		}
		return style.White(strings.Join(items, "  ·  "))
	}

	fmt.Fprintln(s.out, "  "+style.White("rules in force"))
	line("mode", style.White(s.modeName())+style.Gray(" — "+modeDesc(s.modeName())))
	line("allow", join(s.cfg.Permissions.Allow, "nothing pre-allowed (mode + prompts govern)"))
	line("deny", join(s.cfg.Permissions.Deny, "no hard denies"))
	line("egress", join(s.cfg.Egress.Allow, "unrestricted (the web gate still applies)"))

	var hooks []string
	if s.cfg.Hooks.PreToolUse != "" {
		hooks = append(hooks, "pre-tool: "+s.cfg.Hooks.PreToolUse)
	}
	if s.cfg.Hooks.Stop != "" {
		hooks = append(hooks, "stop: "+s.cfg.Hooks.Stop)
	}
	line("hooks", join(hooks, "none"))
}
