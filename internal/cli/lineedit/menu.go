package lineedit

import (
	"sort"
	"strings"
)

// Command is one selectable slash command in the dropdown (mirrors the cli
// slashCommands table — passed in so there is one source of truth).
type Command struct {
	Name, Args, Desc string
}

// slashMenu is the live "/" dropdown state: it opens the instant the buffer is a
// bare "/<prefix>" (no space yet), filters by prefix on every keystroke, and
// supports wrap navigation + completion. Pure state; the editor renders it.
type slashMenu struct {
	commands []Command
	filtered []Command
	sel      int
	open     bool
}

func newSlashMenu(cmds []Command) *slashMenu { return &slashMenu{commands: cmds} }

func (m *slashMenu) reset() {
	m.open, m.sel, m.filtered = false, 0, nil
}

// update recomputes open/filtered/sel from the current buffer. The menu is open
// only while the buffer is a slash token with no space (a space means the
// command is chosen and the user is typing arguments).
func (m *slashMenu) update(buf string) {
	m.open = strings.HasPrefix(buf, "/") && !strings.Contains(buf, " ")
	if !m.open {
		m.filtered, m.sel = nil, 0
		return
	}
	// Fuzzy-match each command against the buffer and rank by score, so "/mdl"
	// finds "/models", a typo'd middle still hits, and exact prefixes stay on top.
	type scored struct {
		c  Command
		sc int
	}
	var hits []scored
	for _, c := range m.commands {
		if sc, _, ok := fuzzyMatch(buf, c.Name); ok {
			hits = append(hits, scored{c, sc})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].sc > hits[j].sc })
	m.filtered = m.filtered[:0]
	for _, h := range hits {
		m.filtered = append(m.filtered, h.c)
	}
	if m.sel >= len(m.filtered) {
		m.sel = 0
	}
}

func (m *slashMenu) down() {
	if n := len(m.filtered); n > 0 {
		m.sel = (m.sel + 1) % n
	}
}

func (m *slashMenu) up() {
	if n := len(m.filtered); n > 0 {
		m.sel = (m.sel - 1 + n) % n
	}
}

// completion returns the buffer the selected command completes to (with a
// trailing space when it takes arguments, so the menu auto-closes), or ok=false
// when there is nothing to complete.
func (m *slashMenu) completion() (string, bool) {
	if !m.open || len(m.filtered) == 0 {
		return "", false
	}
	c := m.filtered[m.sel]
	if c.Args != "" {
		return c.Name + " ", true
	}
	return c.Name, true
}
