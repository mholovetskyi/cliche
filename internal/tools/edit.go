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

	return "", ErrNotFound
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
