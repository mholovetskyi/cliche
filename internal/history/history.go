// Package history is the Context Ledger: it keeps the conversation transcript
// bounded, compacting older turns when it grows too large — but never
// silently. Every compaction is recorded (to the agent's ledger by the
// caller) and the pre-compaction transcript is retained so it can be
// recovered. Compaction only happens at safe task boundaries so it can never
// orphan a tool_use from its tool_result (which would corrupt the transcript).
package history

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mholovetskyi/cliche/internal/provider"
)

// Stats summarizes compaction activity.
type Stats struct {
	Compactions    int
	PrunedMessages int
}

// Manager bounds a transcript to an estimated token budget.
type Manager struct {
	limitTokens int
	keepRecent  int
	stats       Stats
	lastFull    []provider.Message // pre-compaction snapshot, for recovery
}

// New returns a Manager that compacts when the estimate exceeds limitTokens,
// always keeping at least keepRecent recent messages.
func New(limitTokens, keepRecent int) *Manager {
	if keepRecent <= 0 {
		keepRecent = 8
	}
	return &Manager{limitTokens: limitTokens, keepRecent: keepRecent}
}

// EstimateTokens is a cheap chars/4 heuristic for the transcript size.
func EstimateTokens(msgs []provider.Message) int {
	chars := 0
	for _, m := range msgs {
		chars += len(m.Text)
		for _, c := range m.ToolCalls {
			chars += len(c.Name)
			for _, v := range c.Args {
				chars += len(v)
			}
		}
		for _, r := range m.ToolResults {
			chars += len(r.Content)
		}
	}
	return chars / 4
}

// Stats returns the running compaction stats.
func (m *Manager) Stats() Stats { return m.stats }

// Reset drops the recoverable snapshot and zeroes stats (used on /clear) so a
// later /recover does not resurrect a cleared task's context.
func (m *Manager) Reset() {
	m.lastFull = nil
	m.stats = Stats{}
}

// isFreshUser reports whether a message is the start of a new task (a plain
// user prompt, not an orphan tool_result). These are the only safe cut points.
func isFreshUser(msg provider.Message) bool {
	return msg.Role == "user" && strings.TrimSpace(msg.Text) != "" && len(msg.ToolResults) == 0
}

// MaybeCompact compacts msgs if it exceeds the token budget. It keeps the
// first task message, replaces the pruned middle with a deterministic summary,
// and keeps the recent tail — cutting only at a fresh task boundary so the
// transcript stays valid. Returns the (possibly) new transcript, whether it
// changed, and a human description.
func (m *Manager) MaybeCompact(msgs []provider.Message) ([]provider.Message, bool, string) {
	if m.limitTokens <= 0 || len(msgs) <= m.keepRecent+2 {
		return msgs, false, ""
	}
	if EstimateTokens(msgs) <= m.limitTokens {
		return msgs, false, ""
	}

	desired := len(msgs) - m.keepRecent
	cut := -1
	for i := desired; i >= 2; i-- { // never cut at 0 (the first task) or 1
		if isFreshUser(msgs[i]) {
			cut = i
			break
		}
	}
	if cut < 2 {
		// No safe boundary to prune (e.g. one long task of tool cycles). Leave
		// it to the budget cap rather than risk corrupting tool pairing.
		return msgs, false, ""
	}

	pruned := append([]provider.Message(nil), msgs[1:cut]...)
	summary := provider.Message{Role: "assistant", Text: summarize(pruned)}
	out := make([]provider.Message, 0, 2+len(msgs)-cut)
	out = append(out, msgs[0], summary)
	out = append(out, msgs[cut:]...)

	// Only treat it as a real compaction if it actually shrinks the transcript.
	// Otherwise (e.g. summarizing a prior summary) we'd spam no-op compactions
	// every turn while never getting under budget.
	if len(out) >= len(msgs) || EstimateTokens(out) >= EstimateTokens(msgs) {
		return msgs, false, ""
	}

	// Keep the LARGEST transcript seen for recovery, so /recover restores the
	// genuinely full history rather than a near-no-op intermediate state.
	if m.lastFull == nil || EstimateTokens(msgs) > EstimateTokens(m.lastFull) {
		m.lastFull = append([]provider.Message(nil), msgs...)
	}

	m.stats.Compactions++
	m.stats.PrunedMessages += len(pruned)
	info := fmt.Sprintf("summarized %d earlier messages (%d→%d), ~%d tokens retained",
		len(pruned), len(msgs), len(out), EstimateTokens(out))
	return out, true, info
}

// Recover returns the transcript as it was before the most recent compaction.
func (m *Manager) Recover() ([]provider.Message, bool) {
	if m.lastFull == nil {
		return nil, false
	}
	return append([]provider.Message(nil), m.lastFull...), true
}

// summarize builds a deterministic, non-silent summary of pruned messages so
// nothing is lost without a trace (the full detail also lives in the ledger).
func summarize(msgs []provider.Message) string {
	toolCounts := map[string]int{}
	fileSet := map[string]bool{}
	tasks := 0
	for _, msg := range msgs {
		if isFreshUser(msg) {
			tasks++
		}
		for _, c := range msg.ToolCalls {
			toolCounts[c.Name]++
			if f, ok := c.Args["file"]; ok && f != "" {
				fileSet[f] = true
			}
		}
	}

	var tools []string
	for name, n := range toolCounts {
		tools = append(tools, fmt.Sprintf("%s×%d", name, n))
	}
	sort.Strings(tools)
	var files []string
	for f := range fileSet {
		files = append(files, f)
	}
	sort.Strings(files)

	var b strings.Builder
	fmt.Fprintf(&b, "[Earlier context compacted by Cliche: %d messages across %d task(s) summarized.", len(msgs), tasks)
	if len(tools) > 0 {
		fmt.Fprintf(&b, " Tools used: %s.", strings.Join(tools, ", "))
	}
	if len(files) > 0 {
		fmt.Fprintf(&b, " Files touched: %s.", strings.Join(files, ", "))
	}
	b.WriteString(" Full detail is recorded in the ledger and is recoverable.]")
	return b.String()
}
