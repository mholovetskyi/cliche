package governor

import (
	"testing"
	"time"
)

func TestMaxTurns(t *testing.T) {
	g := New(Limits{MaxTurns: 3})
	for i := 0; i < 3; i++ {
		if _, h := g.BeginTurn(); h != nil {
			t.Fatalf("unexpected halt on turn %d: %v", i+1, h)
		}
	}
	_, h := g.BeginTurn() // turn 4
	if h == nil || h.Code != "max_turns" {
		t.Fatalf("expected max_turns halt, got %v", h)
	}
}

func TestRepetitionBreaker(t *testing.T) {
	g := New(Limits{RepetitionWindow: 8, RepetitionThreshold: 3})
	g.BeginTurn()
	if h := g.RecordToolCall("apply_diff:same"); h != nil {
		t.Fatalf("halt too early (1): %v", h)
	}
	if h := g.RecordToolCall("apply_diff:same"); h != nil {
		t.Fatalf("halt too early (2): %v", h)
	}
	h := g.RecordToolCall("apply_diff:same")
	if h == nil || h.Code != "repetition" {
		t.Fatalf("expected repetition halt, got %v", h)
	}
}

func TestRepetitionIgnoresVariedCalls(t *testing.T) {
	g := New(Limits{RepetitionWindow: 8, RepetitionThreshold: 3})
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		if h := g.RecordToolCall("read_file:" + s); h != nil {
			t.Fatalf("varied calls should not trip repetition, got %v", h)
		}
	}
}

func TestFailedEdits(t *testing.T) {
	g := New(Limits{MaxConsecutiveFailedEdits: 3})
	if h := g.RecordEdit(false); h != nil {
		t.Fatalf("halt too early (1): %v", h)
	}
	g.RecordEdit(false)
	h := g.RecordEdit(false)
	if h == nil || h.Code != "failed_edits" {
		t.Fatalf("expected failed_edits halt, got %v", h)
	}
}

func TestFailedEditsResetOnSuccess(t *testing.T) {
	g := New(Limits{MaxConsecutiveFailedEdits: 2})
	g.RecordEdit(false)
	g.RecordEdit(true) // reset
	if h := g.RecordEdit(false); h != nil {
		t.Fatalf("counter should have reset, got %v", h)
	}
}

func TestNoProgress(t *testing.T) {
	g := New(Limits{NoProgressTurns: 3})
	g.RecordTurnProgress(false)
	g.RecordTurnProgress(false)
	h := g.RecordTurnProgress(false)
	if h == nil || h.Code != "no_progress" {
		t.Fatalf("expected no_progress halt, got %v", h)
	}
}

func TestWallClock(t *testing.T) {
	now := time.Unix(0, 0)
	g := New(Limits{MaxWallClock: time.Minute}).WithClock(func() time.Time { return now })
	if _, h := g.BeginTurn(); h != nil {
		t.Fatalf("unexpected halt at t=0: %v", h)
	}
	now = now.Add(2 * time.Minute)
	_, h := g.BeginTurn()
	if h == nil || h.Code != "max_wallclock" {
		t.Fatalf("expected max_wallclock halt, got %v", h)
	}
}
