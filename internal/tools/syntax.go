package tools

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/scanner"
	"go/token"
	"path/filepath"
	"strings"
)

// validateSyntax does a language-aware (AST) sanity check on the new contents
// of a file before it is written, so the agent can't silently break the build.
// v0 validates Go via the standard library's parser (zero dependencies);
// other languages pass through (extensible). Returns an error describing the
// syntax problem, or nil.
func validateSyntax(path, content string) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		fset := token.NewFileSet()
		if _, err := parser.ParseFile(fset, path, content, parser.SkipObjectResolution); err != nil {
			return fmt.Errorf("resulting Go file does not parse: %v%s", err, goParseHint(content, err))
		}
	case ".json":
		if !json.Valid([]byte(content)) {
			return fmt.Errorf("resulting JSON is not valid")
		}
	}
	return nil
}

// goParseHint turns a parser error into actionable feedback: the actual source
// line(s) the error points at, so the agent can SEE what its edit broke instead
// of just a line:col it has to imagine. Bare line numbers are nearly useless to
// a model that can't recount the file; the offending text is what it needs.
func goParseHint(content string, err error) string {
	list, ok := err.(scanner.ErrorList)
	if !ok || len(list) == 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	var b strings.Builder
	seen := map[int]bool{}
	for _, e := range list {
		ln := e.Pos.Line
		if ln < 1 || ln > len(lines) || seen[ln] {
			continue
		}
		seen[ln] = true
		fmt.Fprintf(&b, "\n  line %d: %s", ln, strings.TrimRight(lines[ln-1], "\r"))
		if len(seen) >= 3 {
			break
		}
	}
	return b.String()
}
