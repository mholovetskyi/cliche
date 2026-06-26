package lineedit

import "strings"

// History is an in-memory ↑/↓ prompt history with a scratch slot, so browsing
// older entries preserves the in-progress line and restores it at the bottom.
// Pure (no I/O), so every transition is unit-tested.
type History struct {
	items   []string
	idx     int // cursor into items; == len(items) means "at the live line"
	scratch string
}

// NewHistory seeds the ring with prior prompts (most recent last).
func NewHistory(seed []string) *History {
	h := &History{items: append([]string(nil), seed...)}
	h.idx = len(h.items)
	return h
}

// Add records a submitted line, skipping empties and consecutive duplicates, and
// resets the browse cursor to the live position.
func (h *History) Add(line string) {
	if line == "" {
		h.idx = len(h.items)
		return
	}
	if n := len(h.items); n == 0 || h.items[n-1] != line {
		h.items = append(h.items, line)
	}
	h.idx = len(h.items)
}

// Suggest returns the suffix that completes prefix from the most recent distinct
// history entry beginning with it — the source for inline ghost-text
// autosuggestion — or "" when nothing extends it. Pure.
func (h *History) Suggest(prefix string) string {
	if prefix == "" {
		return ""
	}
	for i := len(h.items) - 1; i >= 0; i-- {
		if e := h.items[i]; len(e) > len(prefix) && strings.HasPrefix(e, prefix) {
			return e[len(prefix):]
		}
	}
	return ""
}

// Prev returns the previous (older) entry. On the first step up from the live
// line, current is saved to scratch so Next can restore it. At the oldest entry
// it stays put.
func (h *History) Prev(current string) string {
	if len(h.items) == 0 {
		return current
	}
	if h.idx == len(h.items) {
		h.scratch = current
	}
	if h.idx > 0 {
		h.idx--
	}
	return h.items[h.idx]
}

// Next walks toward the present; stepping past the newest entry restores the
// scratch (the line the user was typing before browsing).
func (h *History) Next() string {
	if h.idx >= len(h.items) {
		return h.scratch
	}
	h.idx++
	if h.idx >= len(h.items) {
		return h.scratch
	}
	return h.items[h.idx]
}
