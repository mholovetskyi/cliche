package tools

import (
	"strings"
	"testing"
)

func TestReadViewWholeFileRoundTrips(t *testing.T) {
	for _, content := range []string{"", "one line no newline", "a\nb\nc\n", "a\nb\nc", "\n"} {
		if got := readView(content, "", ""); got != content {
			t.Fatalf("whole-file read must round-trip exactly: in=%q out=%q", content, got)
		}
	}
}

func TestReadViewOffsetLimit(t *testing.T) {
	content := "l1\nl2\nl3\nl4\nl5\n"
	// offset=2, limit=2 → lines 2-3.
	got := readView(content, "2", "2")
	if !strings.HasPrefix(got, "l2\nl3") {
		t.Fatalf("offset/limit slice wrong: %q", got)
	}
	if !strings.Contains(got, "showing lines 2-3 of 5") {
		t.Fatalf("partial read should be annotated: %q", got)
	}
	// offset past the relevant range, read to end → no "more" hint.
	got = readView(content, "4", "")
	if !strings.Contains(got, "l4\nl5") || strings.Contains(got, "read the rest") {
		t.Fatalf("reading to end should not advertise more: %q", got)
	}
}

func TestReadViewDefaultCapTruncatesLargeFile(t *testing.T) {
	var b strings.Builder
	for i := 0; i < defaultReadLines+50; i++ {
		b.WriteString("x\n")
	}
	got := readView(b.String(), "", "")
	if !strings.Contains(got, "file is large") {
		t.Fatalf("a file over the cap should be truncated with guidance: tail=%q", got[len(got)-80:])
	}
	// Body should be capped near defaultReadLines, not the full file.
	if lines := strings.Count(got, "x\n"); lines > defaultReadLines+1 {
		t.Fatalf("default read should cap lines, got %d", lines)
	}
}

func TestBoundOutput(t *testing.T) {
	small := "short output"
	if got := boundOutput(small, runOutputLimit); got != small {
		t.Fatalf("small output must pass through unchanged")
	}
	big := strings.Repeat("A", 10) + strings.Repeat("B", 100_000) + strings.Repeat("Z", 10)
	got := boundOutput(big, runOutputLimit)
	if len(got) > runOutputLimit+200 {
		t.Fatalf("bounded output should be near the limit, got %d bytes", len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("bounded output should announce truncation: %q", got[:80])
	}
	// Keeps the head and the tail (where errors usually are).
	if !strings.HasPrefix(got, "AAAA") || !strings.HasSuffix(got, "ZZZZZZZZZZ") {
		t.Fatalf("bounded output should keep head and tail")
	}
}
