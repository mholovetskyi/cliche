package history

import (
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/provider"
)

// buildTranscript makes: task0, [tool cycle]*c0, task1, [tool cycle]*c1, ...
func cycle(file string) []provider.Message {
	return []provider.Message{
		{Role: "assistant", ToolCalls: []provider.ToolCall{{ID: "t", Name: "edit_file", Args: map[string]string{"file": file}}}},
		{Role: "user", ToolResults: []provider.ToolResult{{ID: "t", Content: strings.Repeat("x", 4000)}}},
	}
}

func TestEstimateTokens(t *testing.T) {
	msgs := []provider.Message{{Role: "user", Text: strings.Repeat("a", 400)}}
	if got := EstimateTokens(msgs); got != 100 {
		t.Fatalf("EstimateTokens = %d, want 100", got)
	}
}

func TestNoCompactionUnderBudget(t *testing.T) {
	m := New(1_000_000, 4)
	msgs := []provider.Message{{Role: "user", Text: "task"}}
	if _, did, _ := m.MaybeCompact(msgs); did {
		t.Fatal("should not compact under budget")
	}
}

func TestCompactsAtTaskBoundaryAndStaysValid(t *testing.T) {
	var msgs []provider.Message
	msgs = append(msgs, provider.Message{Role: "user", Text: "task zero"})
	msgs = append(msgs, cycle("a.go")...)
	msgs = append(msgs, cycle("b.go")...)
	msgs = append(msgs, provider.Message{Role: "user", Text: "task one"}) // fresh boundary
	msgs = append(msgs, cycle("c.go")...)

	before := EstimateTokens(msgs)
	m := New(before/2, 2) // force compaction
	out, did, info := m.MaybeCompact(msgs)
	if !did {
		t.Fatalf("expected compaction; info=%q est=%d", info, before)
	}
	if EstimateTokens(out) >= before {
		t.Fatal("compaction did not reduce the estimate")
	}
	// Structure: first message is the original task, second is the summary.
	if out[0].Role != "user" || out[0].Text != "task zero" {
		t.Fatalf("first message should be the original task, got %+v", out[0])
	}
	if out[1].Role != "assistant" || !strings.Contains(out[1].Text, "compacted") {
		t.Fatalf("second message should be the summary, got %+v", out[1])
	}
	// Validity: no orphan tool_result must lead a kept segment, and every
	// tool_result count is covered (no dangling tool_use).
	calls, results := 0, 0
	for _, mm := range out {
		calls += len(mm.ToolCalls)
		results += len(mm.ToolResults)
	}
	if results < calls {
		t.Fatalf("compaction left a dangling tool_use: %d calls, %d results", calls, results)
	}
}

func TestNoSafeBoundaryDoesNotCompact(t *testing.T) {
	// A single long task of tool cycles: no fresh-user boundary to cut at.
	var msgs []provider.Message
	msgs = append(msgs, provider.Message{Role: "user", Text: "one big task"})
	for i := 0; i < 8; i++ {
		msgs = append(msgs, cycle("f.go")...)
	}
	m := New(10, 2) // tiny budget, but nowhere safe to cut
	if _, did, _ := m.MaybeCompact(msgs); did {
		t.Fatal("must not compact when there is no safe task boundary")
	}
}

func TestRepeatedCompactionDoesNotSpam(t *testing.T) {
	var msgs []provider.Message
	msgs = append(msgs, provider.Message{Role: "user", Text: "task zero"})
	msgs = append(msgs, cycle("a.go")...)
	msgs = append(msgs, cycle("b.go")...)
	msgs = append(msgs, provider.Message{Role: "user", Text: "task one"})
	msgs = append(msgs, cycle("c.go")...)

	m := New(EstimateTokens(msgs)/2, 2)
	out, did, _ := m.MaybeCompact(msgs)
	if !did {
		t.Fatal("expected first compaction")
	}
	// Compacting the already-compacted transcript again, still "over budget",
	// must be a no-op (it can only summarize a prior summary -> no shrink).
	out2, did2, _ := m.MaybeCompact(out)
	if did2 {
		t.Fatalf("second compaction should be a no-op, but reported success (len %d->%d)", len(out), len(out2))
	}
	if m.Stats().Compactions != 1 {
		t.Fatalf("compaction count should stay 1, got %d", m.Stats().Compactions)
	}
}

func TestRecover(t *testing.T) {
	var msgs []provider.Message
	msgs = append(msgs, provider.Message{Role: "user", Text: "task zero"})
	msgs = append(msgs, cycle("a.go")...)
	msgs = append(msgs, provider.Message{Role: "user", Text: "task one"})
	msgs = append(msgs, cycle("b.go")...)

	m := New(EstimateTokens(msgs)/2, 2)
	out, did, _ := m.MaybeCompact(msgs)
	if !did {
		t.Fatal("expected compaction")
	}
	full, ok := m.Recover()
	if !ok || len(full) != len(msgs) {
		t.Fatalf("recover should restore %d messages, got %d (ok=%v)", len(msgs), len(full), ok)
	}
	if len(out) >= len(full) {
		t.Fatal("compacted transcript should be shorter than the recovered one")
	}
}
