package cli

import (
	"os"
	"path/filepath"
	"strings"
)

// User-defined extensions (custom slash commands and skills) are plain Markdown
// files under .cliche/, optionally with a simple "---" frontmatter header. This
// file holds the shared frontmatter parser and scaffolding helper — stdlib only,
// no YAML dependency.

// parseFrontmatter splits an optional leading "---" / "---" header from a
// Markdown body, returning the scalar fields (lowercased keys) and the body.
// Only simple "key: value" lines are recognized; anything else passes through as
// body. A file with no header returns an empty map and the whole content.
func parseFrontmatter(content string) (map[string]string, string) {
	meta := map[string]string{}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r ") != "---" {
		return meta, content
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r ") == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return meta, content // no closing fence — treat it all as body
	}
	for _, ln := range lines[1:end] {
		ln = strings.TrimRight(ln, "\r")
		if i := strings.Index(ln, ":"); i > 0 {
			k := strings.ToLower(strings.TrimSpace(ln[:i]))
			meta[k] = strings.Trim(strings.TrimSpace(ln[i+1:]), `"'`)
		}
	}
	body := strings.Join(lines[end+1:], "\n")
	return meta, strings.TrimLeft(body, "\r\n")
}

// scaffold writes content to path (creating parent dirs), refusing to overwrite
// an existing file. Returns whether it created the file and any error.
func scaffold(path, content string) (created bool, err error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil // already exists — never clobber
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
