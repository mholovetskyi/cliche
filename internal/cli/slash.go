package cli

import (
	"fmt"
	"strings"

	"github.com/mholovetskyi/cliche/internal/style"
)

// slashCmd is one in-session command. This table is the single source of truth
// for both /help and the startup hint, so they can't drift.
type slashCmd struct {
	name, args, desc, group string
}

var slashCommands = []slashCmd{
	{"/status", "", "mode, budget, context & guardrails at a glance", "session"},
	{"/cost", "", "spend so far vs the cap", "session"},
	{"/context", "", "context usage vs the limit", "session"},
	{"/insights", "", "usage & spend report", "session"},
	{"/rules", "", "active allow/deny rules, egress & hooks", "session"},
	{"/permissions", "", "inspect allow/deny rules, egress & hooks", "session"},
	{"/plan", "<task>", "add a task to the session plan", "session"},
	{"/tasks", "", "show the session plan", "session"},
	{"/done", "<id>", "mark a plan task complete", "session"},
	{"/sessions", "", "list saved sessions", "control"},
	{"/new", "", "start a fresh session", "control"},
	{"/fork", "", "branch the conversation into a new session", "control"},
	{"/resume", "[id]", "resume a saved session (latest if omitted)", "control"},
	{"/kill", "<id>", "delete a saved session", "control"},
	{"/browse", "", "browse sessions full-screen (mouse + arrows)", "control"},
	{"/dash", "", "full-screen multi-pane dashboard (trust · tasks · changes)", "control"},
	{"/skills", "", "list installed skills", "control"},
	{"/skill", "<name>", "run a skill", "control"},
	{"/commands", "", "list custom commands", "control"},
	{"/plugins", "", "list installed plugins", "control"},
	{"/mcp", "", "list configured MCP servers", "control"},
	{"/memory", "", "show what the agent remembers about this project", "control"},
	{"/bug", "[note]", "write a bug report", "control"},
	{"/diff", "", "changes made this session", "review"},
	{"/changes", "", "browse + revert file changes (full-screen)", "review"},
	{"/undo", "", "revert the last edit", "review"},
	{"/rewind", "", "undo every edit this session", "review"},
	{"/commit", "[msg]", "git commit the work", "review"},
	{"/verify", "", "re-run the project tests", "review"},
	{"/model", "[id]", "show or switch the model", "control"},
	{"/models", "", "list priced models (switch with /model)", "control"},
	{"/theme", "[name]", "show or switch the UI palette", "control"},
	{"/mode", "[name]", "show or switch permission mode", "control"},
	{"/clear", "", "reset the conversation context", "control"},
	{"/recover", "", "undo the last context compaction", "control"},
	{"/help", "", "show this list", "control"},
	{"/exit", "", "quit (also Ctrl-D)", "control"},
}

var slashGroups = []struct{ key, title string }{
	{"session", "session"}, {"review", "review changes"}, {"control", "control"},
}

// help prints the full command palette.
func (s *session) help() { s.commandPalette("") }

// commandPalette renders a dropdown-style box of slash commands — grouped, each
// with a one-line explainer — filtered to those starting with prefix (empty =
// all). It is shown when you type "/" (or any ambiguous prefix like "/c") and by
// /help, so the command set is discoverable at the point of use. A controls
// footer surfaces the keyboard shortcuts the cooked-mode REPL supports.
func (s *session) commandPalette(prefix string) {
	var body strings.Builder
	for _, g := range slashGroups {
		var rows []string
		for _, c := range slashCommands {
			if c.group != g.key || !strings.HasPrefix(c.name, prefix) {
				continue
			}
			name := c.name
			if c.args != "" {
				name += " " + c.args
			}
			rows = append(rows, style.White(style.Pad(name, 18))+style.Gray(c.desc))
		}
		if len(rows) == 0 {
			continue
		}
		if body.Len() > 0 {
			body.WriteByte('\n') // a blank spacer row between groups
		}
		body.WriteString(style.Dim(g.title) + "\n" + strings.Join(rows, "\n"))
	}
	// Append the user's custom commands as their own group.
	var custom []string
	for _, c := range sortedCommands(s.customCmds) {
		if strings.HasPrefix(c.Name, prefix) {
			custom = append(custom, style.White(style.Pad(c.Name, 18))+style.Gray(c.Desc))
		}
	}
	if len(custom) > 0 {
		if body.Len() > 0 {
			body.WriteByte('\n')
		}
		body.WriteString(style.Dim("custom") + "\n" + strings.Join(custom, "\n"))
	}
	if body.Len() == 0 {
		fmt.Fprintf(s.out, "  no command matches %s\n", style.White(prefix))
		return
	}
	fmt.Fprintln(s.out, style.Indent(style.Box("commands", body.String(), style.GrayRGB)))
	fmt.Fprintln(s.out, "  "+style.Dim("Ctrl-C cancel · Ctrl-D exit · \\ continue line · @ attach file · / commands"))
}

// slashHint is the one-line discoverability strip shown at session start.
func slashHint() string {
	names := make([]string, len(slashCommands))
	for i, c := range slashCommands {
		names[i] = c.name
	}
	return strings.Join(names, " · ")
}

// isCommand reports whether name is exactly a known slash command (including the
// /quit alias of /exit, which the dispatch handles but the table omits).
func isCommand(name string) bool {
	for _, c := range slashCommands {
		if c.name == name {
			return true
		}
	}
	return name == "/quit"
}

// prefixMatches returns every command name that input is a prefix of.
func prefixMatches(input string) []string {
	var out []string
	for _, c := range slashCommands {
		if strings.HasPrefix(c.name, input) {
			out = append(out, c.name)
		}
	}
	return out
}

// expandPrefix returns the single command input unambiguously abbreviates
// (/s → /status, /di → /diff), or "" when it matches zero or several commands.
// This is what lets unambiguous abbreviations execute directly, while ambiguous
// ones (/c → cost|context|clear|commit) fall through to a disambiguation hint.
func expandPrefix(input string) string {
	if m := prefixMatches(input); len(m) == 1 {
		return m[0]
	}
	return ""
}

// closestCommand suggests the command a typo most likely meant: a unique prefix
// match, else the nearest by edit distance (≤1). Returns "" when nothing is
// close enough (so we don't guess wildly).
func closestCommand(input string) string {
	if g := expandPrefix(input); g != "" {
		return g
	}
	best, bestD := "", 2
	for _, c := range slashCommands {
		if d := editDistance(c.name, input); d < bestD {
			best, bestD = c.name, d
		}
	}
	if bestD <= 1 {
		return best
	}
	return ""
}

// promptTips rotate at the idle prompt to teach features without nagging.
var promptTips = []string{
	"tip · /status shows budget, mode & guardrails at a glance",
	"tip · /diff reviews changes · /undo reverts the last edit",
	"tip · /mode plan makes the agent read-only",
	"tip · /rules shows what's allowed, denied & where it can reach",
}

// promptTipEvery is how often (in prompts shown) a rotating tip appears.
const promptTipEvery = 5

// promptTip returns a rotating tip for the i-th prompt shown, or "" most of the
// time (the first prompt and the gaps between rotations) so it stays subtle.
func promptTip(i int) string {
	if i <= 0 || i%promptTipEvery != 0 {
		return ""
	}
	return promptTips[(i/promptTipEvery-1)%len(promptTips)]
}

// editDistance is the Levenshtein distance (small inputs; simple DP).
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
