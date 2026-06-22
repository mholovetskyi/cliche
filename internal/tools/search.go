package tools

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Read-only discovery tools (search_files / find_files / list_files) let the
// model locate code without shelling out to grep/find — which would be slow,
// platform-dependent, and (via run_command) gated behind a shell permission.
// These are pure Go, confined to the project root exactly like read_file, and
// every result set is bounded so a search can never flood the context window.

const (
	maxSearchMatches  = 200     // cap matched lines returned by search_files
	maxFindResults    = 1000    // cap paths returned by find_files
	maxListEntries    = 1000    // cap entries returned by list_files
	maxSearchFileSize = 5 << 20 // skip files larger than 5 MiB when searching
	maxMatchLineLen   = 300     // truncate long matched lines
	binarySniffBytes  = 8000    // bytes inspected for a NUL when detecting binary
)

// skipDirs are never descended into by search_files / find_files. They are
// noise for a coding agent (VCS internals, fetched dependencies, build output).
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
}

// errStopWalk is a sentinel used to abort a filepath.WalkDir early once a result
// cap is reached (WalkDir treats any non-nil error as "stop").
var errStopWalk = errors.New("stop")

// searchFiles greps the project for a regular expression, returning matching
// lines as "relpath:lineno: text". It is confined to the project root, skips
// binary and oversized files, and bounds the number of matches it returns.
func (e OSExecutor) searchFiles(args map[string]string) Result {
	pattern := strings.TrimSpace(args["pattern"])
	if pattern == "" {
		return Result{Output: "search error: no pattern specified", Success: false}
	}
	expr := pattern
	if args["ignore_case"] == "true" {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return Result{Output: "search error: invalid regex: " + err.Error(), Success: false}
	}
	root, base, err := e.walkRoot(args["path"])
	if err != nil {
		return Result{Output: "search denied: " + err.Error(), Success: false}
	}
	glob := strings.TrimSpace(args["glob"])

	var matches []string
	truncated := false
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries rather than aborting the whole walk
		}
		if d.IsDir() {
			if p != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // don't follow symlinks or read devices/sockets
		}
		rel := relSlash(base, root, p)
		if glob != "" && !pathMatchesGlob(glob, rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxSearchFileSize {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil || isBinary(data) {
			return nil
		}
		text := string(data)
		lines := strings.Split(text, "\n")
		if strings.HasSuffix(text, "\n") {
			lines = lines[:len(lines)-1] // a terminating newline doesn't start a new line
		}
		for n, line := range lines {
			if re.MatchString(line) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", rel, n+1, clipLine(line)))
				if len(matches) >= maxSearchMatches {
					truncated = true
					return errStopWalk
				}
			}
		}
		return nil
	})

	if len(matches) == 0 {
		return Result{Output: "no matches for " + pattern, Success: true}
	}
	out := fmt.Sprintf("%d match(es):\n%s", len(matches), strings.Join(matches, "\n"))
	if truncated {
		out += fmt.Sprintf("\n(results truncated at %d matches — narrow the pattern or set a path/glob)", maxSearchMatches)
	}
	return Result{Output: out, Success: true}
}

// findFiles lists files whose path matches a glob pattern (supporting **), one
// per line, confined to the project root and bounded.
func (e OSExecutor) findFiles(args map[string]string) Result {
	glob := strings.TrimSpace(args["pattern"])
	if glob == "" {
		return Result{Output: "find error: no pattern specified", Success: false}
	}
	root, base, err := e.walkRoot(args["path"])
	if err != nil {
		return Result{Output: "find denied: " + err.Error(), Success: false}
	}

	var found []string
	truncated := false
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel := relSlash(base, root, p)
		if pathMatchesGlob(glob, rel) {
			found = append(found, rel)
			if len(found) >= maxFindResults {
				truncated = true
				return errStopWalk
			}
		}
		return nil
	})

	if len(found) == 0 {
		return Result{Output: "no files match " + glob, Success: true}
	}
	out := fmt.Sprintf("%d file(s):\n%s", len(found), strings.Join(found, "\n"))
	if truncated {
		out += fmt.Sprintf("\n(truncated at %d files)", maxFindResults)
	}
	return Result{Output: out, Success: true}
}

// listFiles lists the immediate entries of a directory (non-recursive),
// marking directories with a trailing slash, confined to the project root.
func (e OSExecutor) listFiles(args map[string]string) Result {
	dir, err := e.resolve(orDot(args["path"]))
	if err != nil {
		return Result{Output: "list denied: " + err.Error(), Success: false}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Result{Output: "list error: " + err.Error(), Success: false}
	}
	names := make([]string, 0, len(entries))
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	truncated := false
	if len(names) > maxListEntries {
		names = names[:maxListEntries]
		truncated = true
	}
	if len(names) == 0 {
		return Result{Output: "(empty directory)", Success: true}
	}
	out := strings.Join(names, "\n")
	if truncated {
		out += fmt.Sprintf("\n(truncated at %d entries)", maxListEntries)
	}
	return Result{Output: out, Success: true}
}

// walkRoot resolves the (optional) path argument under confinement and returns
// the absolute root to walk plus the original (possibly relative) base, so
// results can be reported relative to what the caller asked about.
func (e OSExecutor) walkRoot(arg string) (root, base string, err error) {
	base = orDot(arg)
	root, err = e.resolve(base)
	return root, base, err
}

// relSlash reports p relative to the search base using forward slashes, so
// reported paths read the same on every platform and echo the caller's input.
func relSlash(base, root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	if base != "." && base != "" {
		rel = filepath.Join(base, rel)
	}
	return filepath.ToSlash(rel)
}

func orDot(s string) string {
	if strings.TrimSpace(s) == "" {
		return "."
	}
	return s
}

// clipLine trims surrounding whitespace and bounds the length of a matched line.
func clipLine(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxMatchLineLen {
		return s[:maxMatchLineLen] + "…"
	}
	return s
}

// isBinary reports whether data looks binary (contains a NUL byte in its head),
// so text search skips it the way ripgrep/grep do by default.
func isBinary(data []byte) bool {
	if len(data) > binarySniffBytes {
		data = data[:binarySniffBytes]
	}
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

// pathMatchesGlob matches a forward-slash relative path against a glob. A
// pattern without a slash matches against the base name at any depth (so "*.go"
// finds every Go file); a pattern with a slash is matched against the full path
// and supports "**" to span directories.
func pathMatchesGlob(glob, rel string) bool {
	rel = filepath.ToSlash(rel)
	if !strings.Contains(glob, "/") {
		ok, _ := path.Match(glob, path.Base(rel))
		return ok
	}
	return globMatch(glob, rel)
}

// globMatch matches a slash-separated path against a slash-separated pattern,
// treating a "**" segment as matching zero or more path segments. Within a
// single segment, standard path.Match wildcards (*, ?, [..]) apply.
func globMatch(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			if len(pat) == 1 {
				return true // trailing ** matches any remaining depth
			}
			for i := 0; i <= len(name); i++ {
				if matchSegments(pat[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], name[0]); err != nil || !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}
