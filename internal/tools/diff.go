package tools

import (
	"fmt"
	"strings"
)

// changePreview renders a compact, bounded preview of replacing oldText with
// newText, for display in an approval prompt. It is honest and self-limiting:
// a brand-new file is summarized by its size, and an edit is shown as a unified
// line diff capped at maxPreviewLines so a large rewrite can never flood the
// terminal. For very large files the line diff is skipped in favor of a size
// summary (the O(n·m) LCS is only worth running on human-sized inputs).
const (
	maxPreviewLines = 40   // most -/+ lines shown in an approval preview
	maxDiffLines    = 1500 // skip the O(n·m) line diff above this size; summarize instead
	previewLineLen  = 200  // truncate any single previewed line to this width
)

func changePreview(oldText, newText string) string {
	if oldText == newText {
		return "(no change)"
	}
	if oldText == "" {
		return fmt.Sprintf("new file: +%d line(s)", countLines(newText))
	}
	if newText == "" {
		return fmt.Sprintf("clears file: -%d line(s)", countLines(oldText))
	}
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")
	if len(oldLines) > maxDiffLines || len(newLines) > maxDiffLines {
		return fmt.Sprintf("overwrite: %d line(s) → %d line(s)", len(oldLines), len(newLines))
	}
	return renderDiff(diffLines(oldLines, newLines))
}

// op is a single line-level diff operation.
type op struct {
	kind byte // ' ' equal, '-' removed, '+' added
	text string
}

// diffLines computes a line-level diff via a longest-common-subsequence DP and
// backtrack. Output order matches the original files (removals before the
// additions that replace them).
func diffLines(a, b []string) []op {
	n, m := len(a), len(b)
	// lcs[i][j] = length of the LCS of a[i:] and b[j:].
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, op{' ', a[i]})
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, op{'-', a[i]})
			i++
		default:
			ops = append(ops, op{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, op{'+', b[j]})
	}
	return ops
}

// renderDiff shows only changed lines (the -/+ ops), the most compact view for
// an approval prompt, capped at maxPreviewLines with a tail summary of what was
// elided so the preview never misrepresents the size of the change.
func renderDiff(ops []op) string {
	var removed, added int
	for _, o := range ops {
		switch o.kind {
		case '-':
			removed++
		case '+':
			added++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d removed / %d added", removed, added)
	shown := 0
	var shownRem, shownAdd int
	for _, o := range ops {
		if o.kind == ' ' {
			continue
		}
		if shown >= maxPreviewLines {
			break
		}
		fmt.Fprintf(&b, "\n    %c %s", o.kind, clipPreview(o.text))
		shown++
		if o.kind == '-' {
			shownRem++
		} else {
			shownAdd++
		}
	}
	if rem := (removed - shownRem) + (added - shownAdd); rem > 0 {
		fmt.Fprintf(&b, "\n    … %d more changed line(s)", rem)
	}
	return b.String()
}

func clipPreview(s string) string {
	s = strings.TrimRight(s, "\r")
	if len(s) > previewLineLen {
		return s[:previewLineLen] + "…"
	}
	return s
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
