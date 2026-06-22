package tools

import (
	"fmt"
	"go/parser"
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
			return fmt.Errorf("resulting Go file does not parse: %v", err)
		}
	}
	return nil
}
