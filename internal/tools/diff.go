package tools

import (
	"fmt"
	"strings"

	"github.com/mholovetskyi/cliche/internal/style"
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
// elided so the preview never misrepresents the size of the change. A single
// line replaced in place (one '-' immediately followed by one '+') gets a
// word-level highlight so the eye lands on exactly what changed.
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
	// Collect just the changed lines, in order.
	var changed []op
	for _, o := range ops {
		if o.kind != ' ' {
			changed = append(changed, o)
		}
	}

	var b strings.Builder
	// Color is applied HERE, where the op kind is known — not by re-parsing the
	// rendered string for a leading -/+ (which mis-tints content that legitimately
	// starts with - or +, e.g. a YAML "- item" or a "--force" flag).
	b.WriteString(style.Gray(fmt.Sprintf("%d removed / %d added", removed, added)))
	i, shown := 0, 0
	for i < len(changed) && shown < maxPreviewLines {
		o := changed[i]
		// An ISOLATED '-' immediately followed by a '+' (no adjacent '-' before or
		// '+' after) is a single-line replacement: highlight only the words that
		// differ. A multi-line block replace falls through to plain lines so we
		// never pair an unrelated removal with an addition.
		isolatedReplace := o.kind == '-' &&
			i+1 < len(changed) && changed[i+1].kind == '+' &&
			(i == 0 || changed[i-1].kind != '-') &&
			(i+2 >= len(changed) || changed[i+2].kind != '+')
		if isolatedReplace {
			oldS, newS := intralineDiff(o.text, changed[i+1].text)
			b.WriteString("\n" + style.Red("    - ") + oldS)
			b.WriteString("\n" + style.Green("    + ") + newS)
			i += 2
			shown += 2
			continue
		}
		line := clipPreview(o.text)
		if o.kind == '-' {
			b.WriteString("\n" + style.Red("    - "+line))
		} else {
			b.WriteString("\n" + style.Green("    + "+line))
		}
		i++
		shown++
	}
	if rem := len(changed) - i; rem > 0 {
		b.WriteString("\n" + style.Gray(fmt.Sprintf("    … %d more changed line(s)", rem)))
	}
	return b.String()
}

// diff token tints: the unchanged parts of a changed line stay a muted version of
// the line color, so the bright bold word(s) that actually differ stand out.
var (
	diffOldDim = style.RGB{R: 152, G: 82, B: 84}
	diffNewDim = style.RGB{R: 96, G: 142, B: 100}
)

// intralineDiff word-diffs a replaced line and returns the two styled halves: the
// shared tokens muted, the differing tokens bright + bold. Content is preserved
// exactly (the escapes carry zero display width); under NO_COLOR it returns the
// two lines plain.
func intralineDiff(oldText, newText string) (string, string) {
	oldC, newC := clipPreview(oldText), clipPreview(newText)
	if !style.Enabled {
		return oldC, newC
	}
	ops := diffLines(tokenize(oldC), tokenize(newC)) // same LCS, over word tokens
	var ob, nb strings.Builder
	for _, o := range ops {
		switch o.kind {
		case ' ':
			ob.WriteString(style.Color(o.text, diffOldDim))
			nb.WriteString(style.Color(o.text, diffNewDim))
		case '-':
			ob.WriteString(style.BoldRed(o.text))
		case '+':
			nb.WriteString(style.BoldGreen(o.text))
		}
	}
	return ob.String(), nb.String()
}

// tokenize splits a line into maximal runs of word vs non-word runes, so a
// word-level diff aligns on words and punctuation rather than characters.
func tokenize(s string) []string {
	rs := []rune(s)
	var toks []string
	for i := 0; i < len(rs); {
		w := isWordRune(rs[i])
		j := i + 1
		for j < len(rs) && isWordRune(rs[j]) == w {
			j++
		}
		toks = append(toks, string(rs[i:j]))
		i = j
	}
	return toks
}

func isWordRune(r rune) bool {
	return r == '_' ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r >= 0x80 // keep multibyte runs grouped
}

func clipPreview(s string) string {
	s = strings.TrimRight(s, "\r")
	rs := []rune(s)
	if len(rs) > previewLineLen {
		return string(rs[:previewLineLen]) + "…" // rune-safe: never cut mid-UTF-8
	}
	return s
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
