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
	{"/cost", "", "spend so far vs the cap", "session"},
	{"/context", "", "context usage vs the limit", "session"},
	{"/diff", "", "changes made this session", "review"},
	{"/undo", "", "revert the last edit", "review"},
	{"/rewind", "", "undo every edit this session", "review"},
	{"/commit", "[msg]", "git commit the work", "review"},
	{"/verify", "", "re-run the project tests", "review"},
	{"/model", "[id]", "show or switch the model", "control"},
	{"/mode", "[name]", "show or switch permission mode", "control"},
	{"/clear", "", "reset the conversation context", "control"},
	{"/recover", "", "undo the last context compaction", "control"},
	{"/help", "", "show this list", "control"},
	{"/exit", "", "quit (also Ctrl-D)", "control"},
}

var slashGroups = []struct{ key, title string }{
	{"session", "session"}, {"review", "review changes"}, {"control", "control"},
}

// help prints the commands grouped, with a Pad-aligned name column (replacing
// the old hand-padded list that misaligned).
func (s *session) help() {
	for _, g := range slashGroups {
		fmt.Fprintln(s.out, "  "+style.Dim(g.title))
		for _, c := range slashCommands {
			if c.group != g.key {
				continue
			}
			name := c.name
			if c.args != "" {
				name += " " + c.args
			}
			fmt.Fprintf(s.out, "    %s %s\n", style.White(style.Pad(name, 14)), style.Gray(c.desc))
		}
	}
}

// slashHint is the one-line discoverability strip shown at session start.
func slashHint() string {
	names := make([]string, len(slashCommands))
	for i, c := range slashCommands {
		names[i] = c.name
	}
	return strings.Join(names, " · ")
}

// closestCommand suggests the command a typo most likely meant: a unique prefix
// match, else the nearest by edit distance (≤1). Returns "" when nothing is
// close enough (so we don't guess wildly).
func closestCommand(input string) string {
	var prefix []string
	for _, c := range slashCommands {
		if strings.HasPrefix(c.name, input) {
			prefix = append(prefix, c.name)
		}
	}
	if len(prefix) == 1 {
		return prefix[0]
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
