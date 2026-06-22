package tools

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/style"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestRenderDiffColorsAtGeneration(t *testing.T) {
	old := style.Enabled
	style.Enabled = true
	defer func() { style.Enabled = old }()

	got := changePreview("one\ntwo\n", "one\nTWO\n")
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("the diff should be colored at generation: %q", got)
	}
	// Removed and added lines must carry DIFFERENT color escapes (red vs green) —
	// additions used to be uncolored (invisible).
	var removedSeq, addedSeq string
	for _, ln := range strings.Split(got, "\n") {
		if plain := ansiRe.ReplaceAllString(ln, ""); strings.Contains(plain, "- two") {
			removedSeq = ansiRe.FindString(ln)
		} else if strings.Contains(plain, "+ TWO") {
			addedSeq = ansiRe.FindString(ln)
		}
	}
	if removedSeq == "" || addedSeq == "" || removedSeq == addedSeq {
		t.Fatalf("removed (%q) and added (%q) lines should be colored differently", removedSeq, addedSeq)
	}
}

func TestChangePreviewNewAndCleared(t *testing.T) {
	if got := changePreview("", "a\nb\nc\n"); !strings.Contains(got, "new file") || !strings.Contains(got, "+4") {
		// "a\nb\nc\n" is 4 lines (trailing newline → empty final line).
		t.Fatalf("new-file preview wrong: %q", got)
	}
	if got := changePreview("x\ny", ""); !strings.Contains(got, "clears file") {
		t.Fatalf("cleared preview wrong: %q", got)
	}
	if got := changePreview("same", "same"); got != "(no change)" {
		t.Fatalf("no-change preview wrong: %q", got)
	}
}

func TestChangePreviewEditShowsAddedAndRemoved(t *testing.T) {
	old := "one\ntwo\nthree\n"
	new := "one\nTWO\nthree\n"
	got := changePreview(old, new)
	if !strings.Contains(got, "1 removed / 1 added") {
		t.Fatalf("expected counts, got:\n%s", got)
	}
	if !strings.Contains(got, "- two") || !strings.Contains(got, "+ TWO") {
		t.Fatalf("expected the changed lines, got:\n%s", got)
	}
	// Unchanged lines are not shown in the compact preview.
	if strings.Contains(got, "one") || strings.Contains(got, "three") {
		t.Fatalf("unchanged lines should be hidden, got:\n%s", got)
	}
}

func TestChangePreviewTruncatesLargeDiff(t *testing.T) {
	var oldB, newB strings.Builder
	for i := 0; i < 200; i++ {
		oldB.WriteString("old\n")
		newB.WriteString("new\n")
	}
	got := changePreview(oldB.String(), newB.String())
	if !strings.Contains(got, "more changed line(s)") {
		t.Fatalf("large diff should be truncated with a tail summary, got:\n%s", got)
	}
	// The body must not exceed the cap (header + capped lines + tail).
	if n := strings.Count(got, "\n"); n > maxPreviewLines+3 {
		t.Fatalf("preview exceeded cap: %d lines", n)
	}
}

func TestChangePreviewHugeFileFallsBackToSummary(t *testing.T) {
	var b strings.Builder
	for i := 0; i < maxDiffLines+10; i++ {
		b.WriteString("line\n")
	}
	got := changePreview(b.String(), b.String()+"extra\n")
	if !strings.Contains(got, "overwrite:") {
		t.Fatalf("oversized diff should summarize, got:\n%s", got)
	}
}

func TestDiffLinesOrdering(t *testing.T) {
	ops := diffLines([]string{"a", "b", "c"}, []string{"a", "x", "c"})
	var b strings.Builder
	for _, o := range ops {
		b.WriteByte(o.kind)
	}
	// equal a, remove b, add x, equal c.
	if b.String() != " -+ " {
		t.Fatalf("unexpected op sequence: %q", b.String())
	}
}
