package ledger

import "testing"

func TestAppendAndSummarize(t *testing.T) {
	l, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = l.Append(Entry{Turn: 1, Event: EventTurn, InputTokens: 100, OutputTokens: 50, USD: 0.01})
	_ = l.Append(Entry{Turn: 2, Event: EventTurn, InputTokens: 200, OutputTokens: 50, USD: 0.02})
	_ = l.Append(Entry{Event: EventVerdict, Verdict: "flagged"})

	s, err := l.Summarize()
	if err != nil {
		t.Fatal(err)
	}
	if s.Turns != 2 {
		t.Fatalf("turns: got %d, want 2", s.Turns)
	}
	if s.InputTokens != 300 || s.OutputTokens != 100 {
		t.Fatalf("tokens: got %d/%d, want 300/100", s.InputTokens, s.OutputTokens)
	}
	if s.Verdicts["flagged"] != 1 {
		t.Fatalf("verdicts: got %v", s.Verdicts)
	}
}

func TestSummarizeMissingFile(t *testing.T) {
	l, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s, err := l.Summarize()
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if s.Turns != 0 {
		t.Fatalf("empty summary expected, got %d turns", s.Turns)
	}
}
