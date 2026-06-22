package tools

import (
	"errors"
	"strings"
)

// Edit errors.
var (
	ErrEmptyOld  = errors.New("old_string is empty")
	ErrNotFound  = errors.New("old_string not found in file")
	ErrAmbiguous = errors.New("old_string matches multiple locations; make it unique or set replace_all")
)

// applyEdit replaces oldStr with newStr in content. It tries an exact match
// first, then a whitespace-tolerant line-block match (the common case where
// the model's snippet differs only in leading/trailing indentation). This is
// the engine that makes targeted edits reliable instead of flaky.
func applyEdit(content, oldStr, newStr string, replaceAll bool) (string, error) {
	if oldStr == "" {
		return "", ErrEmptyOld
	}

	// 1) Exact match.
	if n := strings.Count(content, oldStr); n > 0 {
		if n > 1 && !replaceAll {
			return "", ErrAmbiguous
		}
		if replaceAll {
			return strings.ReplaceAll(content, oldStr, newStr), nil
		}
		return strings.Replace(content, oldStr, newStr, 1), nil
	}

	// 2) Whitespace-tolerant contiguous line-block match.
	if out, ok := editByTrimmedLines(content, oldStr, newStr, replaceAll); ok {
		return out, nil
	}

	// 3) Anchored fuzzy match (single replacement only): locate the block by
	// similarity. Applied only when one candidate is BOTH above threshold AND
	// clearly the best — we bias hard toward refusing over a wrong-location edit.
	if !replaceAll {
		if out, ok := editByFuzzyAnchor(content, oldStr, newStr); ok {
			return out, nil
		}
	}

	return "", ErrNotFound
}

// fuzzy match thresholds: a candidate must score at least matchThreshold and
// beat the runner-up by uniqueMargin, or we refuse.
const (
	matchThreshold = 0.75
	uniqueMargin   = 0.10
)

// editByFuzzyAnchor finds the line-window most similar to oldStr and replaces
// it, but only if the best match is confident, unambiguous, AND has a real
// exact anchor line. It deliberately refuses single-line targets: with no
// surrounding context, a "similar" single line is just a wrong-location edit
// waiting to happen (false edits are catastrophic for this product).
func editByFuzzyAnchor(content, oldStr, newStr string) (string, bool) {
	contentLines := strings.Split(content, "\n")
	oldTrimmed := trimAll(strings.Split(strings.Trim(oldStr, "\n"), "\n"))
	n := len(oldTrimmed)
	if n < 2 || n > len(contentLines) {
		return "", false
	}

	best, second := -1.0, -1.0
	bestIdx := -1
	for i := 0; i+n <= len(contentLines); i++ {
		s := blockScore(contentLines[i:i+n], oldTrimmed)
		if s > best {
			second, best, bestIdx = best, s, i
		} else if s > second {
			second = s
		}
	}
	if bestIdx < 0 || best < matchThreshold || best-second < uniqueMargin {
		return "", false
	}
	// Require at least one EXACT (trimmed) anchor line in the chosen block, so a
	// fuzzy match is always pinned to a real, unambiguous landmark.
	if !hasExactAnchor(contentLines[bestIdx:bestIdx+n], oldTrimmed) {
		return "", false
	}

	newLines := strings.Split(newStr, "\n")
	out := make([]string, 0, len(contentLines)-n+len(newLines))
	out = append(out, contentLines[:bestIdx]...)
	out = append(out, newLines...)
	out = append(out, contentLines[bestIdx+n:]...)
	return strings.Join(out, "\n"), true
}

// hasExactAnchor reports whether any non-empty line in the window matches the
// target exactly (after trimming) — the landmark a fuzzy edit pins to.
func hasExactAnchor(window, oldTrimmed []string) bool {
	for i := range oldTrimmed {
		if oldTrimmed[i] != "" && strings.TrimSpace(window[i]) == oldTrimmed[i] {
			return true
		}
	}
	return false
}

// blockScore is the mean per-line similarity between a window and the target.
func blockScore(window, oldTrimmed []string) float64 {
	total := 0.0
	for i := range oldTrimmed {
		total += lineSimilarity(strings.TrimSpace(window[i]), oldTrimmed[i])
	}
	return total / float64(len(oldTrimmed))
}

// lineSimilarity is 1 - normalized edit distance (1.0 == identical). The
// denominator is in RUNES to match levenshtein (which is rune-based), so
// multibyte text isn't scored as falsely similar.
func lineSimilarity(a, b string) float64 {
	if a == b {
		return 1
	}
	m := len([]rune(a))
	if rl := len([]rune(b)); rl > m {
		m = rl
	}
	if m == 0 {
		return 1
	}
	return 1 - float64(levenshtein(a, b))/float64(m)
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur := make([]int, lb+1)
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// editByTrimmedLines finds a contiguous run of lines whose trimmed forms equal
// the trimmed lines of oldStr, and replaces that run with newStr.
func editByTrimmedLines(content, oldStr, newStr string, replaceAll bool) (string, bool) {
	contentLines := strings.Split(content, "\n")
	oldLines := trimAll(strings.Split(strings.Trim(oldStr, "\n"), "\n"))
	if len(oldLines) == 0 {
		return "", false
	}

	matches := findBlockMatches(contentLines, oldLines)
	if len(matches) == 0 {
		return "", false
	}
	if len(matches) > 1 && !replaceAll {
		return "", false // ambiguous under normalization -> treat as no safe match
	}

	newLines := strings.Split(newStr, "\n")
	// Replace from the last match backwards so earlier indices stay valid.
	for i := len(matches) - 1; i >= 0; i-- {
		start := matches[i]
		end := start + len(oldLines)
		rebuilt := make([]string, 0, len(contentLines)-len(oldLines)+len(newLines))
		rebuilt = append(rebuilt, contentLines[:start]...)
		rebuilt = append(rebuilt, newLines...)
		rebuilt = append(rebuilt, contentLines[end:]...)
		contentLines = rebuilt
		if !replaceAll {
			break
		}
	}
	return strings.Join(contentLines, "\n"), true
}

func findBlockMatches(contentLines, oldTrimmed []string) []int {
	var starts []int
	for i := 0; i+len(oldTrimmed) <= len(contentLines); i++ {
		ok := true
		for j := range oldTrimmed {
			if strings.TrimSpace(contentLines[i+j]) != oldTrimmed[j] {
				ok = false
				break
			}
		}
		if ok {
			starts = append(starts, i)
			i += len(oldTrimmed) - 1 // only non-overlapping matches, so backward rebuild stays valid
		}
	}
	return starts
}

func trimAll(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimSpace(l)
	}
	return out
}
