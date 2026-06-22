package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/style"
)

// outcomeMetrics carries the per-mode numbers shown beside an outcome.
type outcomeMetrics struct {
	elapsed    time.Duration // 0 = omit
	tokens     int           // total tokens to display
	taskUSD    float64       // cost of this task / run
	sessionUSD float64       // running session total; < 0 = omit (one-shot run)
}

// renderOutcome is the single end-of-task summary shared by chat and run, so
// both modes speak one visual language: an icon + status header, a humanized
// remedy for any halt, a cost line, and the verdict. Indented on the gutter
// (the done/stop lines used to print flush-left, breaking the rhythm).
func renderOutcome(out io.Writer, o agent.Outcome, m outcomeMetrics) {
	icon, label, paint := outcomeBadge(o.Stop)
	meta := pluralTurns(o.Turns)
	if m.elapsed > 0 {
		meta += " · " + humanDuration(m.elapsed)
	}
	fmt.Fprintf(out, "\n  %s %s\n", paint(icon+" "+label), style.Gray("· "+meta))

	if h := humanStop(o); h != "" && o.Stop != agent.StopCompleted {
		fmt.Fprintln(out, "  "+style.Gray(h))
	}

	cost := fmt.Sprintf("~%s tokens · $%.4f", humanTokens(m.tokens), m.taskUSD)
	if m.sessionUSD >= 0 {
		cost = fmt.Sprintf("~%s tokens · $%.4f this task · $%.4f session", humanTokens(m.tokens), m.taskUSD, m.sessionUSD)
	}
	fmt.Fprintln(out, "  "+style.Gray(cost))

	if o.Verdict != "" {
		fmt.Fprintln(out, "  "+verdictStyled(o.Verdict))
	}
}

// outcomeBadge returns the icon, label, and color for a stop state.
func outcomeBadge(stop string) (icon, label string, paint func(string) string) {
	switch stop {
	case agent.StopCompleted:
		return gl("✓", "[ok]"), "done", style.BoldGreen
	case agent.StopCancelled:
		return gl("■", "[x]"), "interrupted", style.Red
	case agent.StopBudget:
		return gl("■", "[!]"), "stopped: budget", style.BoldRed
	default:
		return gl("■", "[!]"), "stopped: " + stop, style.BoldRed
	}
}

// humanStop turns a raw governor/stop code into a plain-English remedy line.
func humanStop(o agent.Outcome) string {
	switch o.Stop {
	case agent.StopBudget:
		return "hit the spend/token cap — raise it with --max-usd / --max-tokens or in config."
	case agent.StopError:
		return o.Reason
	case "max_turns":
		return "reached the turn limit — raise governor.max_turns if the task genuinely needs more."
	case "max_wallclock":
		return "ran past the wall-clock limit — raise governor.max_wallclock_seconds."
	case "repetition":
		return "the agent was repeating itself — a loop the governor broke."
	case "failed_edits":
		return "too many edits failed in a row — the target text may have moved; try rephrasing."
	case "no_progress":
		return "stopped making progress — the task may be stuck; rephrase or break it up."
	case agent.StopCancelled:
		return "" // self-evident
	default:
		return o.Reason
	}
}

func pluralTurns(n int) string {
	if n == 1 {
		return "1 turn"
	}
	return fmt.Sprintf("%d turns", n)
}

func humanDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return "<1s"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()+0.5))
	default:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm%02ds", m, int(d.Seconds())-m*60)
	}
}

func humanTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

// boundMessage caps an error body so a JSON/stack blob can't shred the block:
// trimmed, ≤ 300 runes, with continuation lines indented onto the gutter.
func boundMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if rs := []rune(msg); len(rs) > 300 {
		msg = string(rs[:300]) + "…"
	}
	return strings.ReplaceAll(msg, "\n", "\n  ")
}
