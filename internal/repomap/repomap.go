// Package repomap builds a compact, dependency-free structural overview of a
// project: a directory tree of source files, annotated with the top-level Go
// symbols (funcs and types) in each Go file. It gives the agent a map of "where
// things are" up front — like Aider's repo map — so it navigates instead of
// blindly listing directories. Go symbols come from go/parser (stdlib); every
// other language is listed by file. The output is bounded so it can be injected
// into the system prompt without blowing the token budget.
package repomap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// skipDirs are never descended into: VCS, deps, build output, and Cliche's own
// state. Keeps the map about the user's source, not vendored noise.
var skipDirs = map[string]bool{
	".git": true, ".cliche": true, "node_modules": true, "vendor": true,
	"dist": true, "build": true, "out": true, "target": true, ".next": true,
	"__pycache__": true, ".venv": true, "venv": true, ".idea": true, ".vscode": true,
}

// sourceExts are the file extensions worth mapping (code + a few configs).
var sourceExts = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
	".rs": true, ".java": true, ".rb": true, ".c": true, ".h": true, ".cpp": true,
	".cs": true, ".php": true, ".swift": true, ".kt": true, ".scala": true,
	".sh": true, ".sql": true,
}

const maxSymbolsPerFile = 8

// Build walks root and returns a bounded repo map (truncated to maxBytes, with a
// note if it overflowed). A nil/zero maxBytes uses a sensible default.
func Build(root string, maxBytes int) (string, error) {
	if maxBytes <= 0 {
		maxBytes = 8000
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}

	// Collect source files grouped by their directory (relative to root).
	byDir := map[string][]string{}
	var dirs []string
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries rather than aborting the whole map
		}
		if d.IsDir() {
			if path != absRoot && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") && d.Name() != ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !sourceExts[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		rel, _ := filepath.Rel(absRoot, path)
		dir := filepath.ToSlash(filepath.Dir(rel))
		entry := d.Name()
		if syms := goSymbols(path); syms != "" {
			entry += "  " + syms
		}
		if _, seen := byDir[dir]; !seen {
			dirs = append(dirs, dir)
		}
		byDir[dir] = append(byDir[dir], entry)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(dirs)

	var b strings.Builder
	truncated := false
	for _, dir := range dirs {
		label := dir + "/"
		if dir == "." {
			label = "./"
		}
		line := label + "\n"
		if b.Len()+len(line) > maxBytes {
			truncated = true
			break
		}
		b.WriteString(line)
		files := byDir[dir]
		sort.Strings(files)
		for _, f := range files {
			fl := "  " + f + "\n"
			if b.Len()+len(fl) > maxBytes {
				truncated = true
				break
			}
			b.WriteString(fl)
		}
		if truncated {
			break
		}
	}
	if truncated {
		b.WriteString("… (map truncated)\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// goSymbols extracts up to maxSymbolsPerFile top-level func/type names from a Go
// file. Returns "" for non-Go files or on parse failure (best effort).
func goSymbols(path string) string {
	if strings.ToLower(filepath.Ext(path)) != ".go" {
		return ""
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if f == nil {
		return "" // unparseable beyond recovery
	}
	_ = err // a partial AST is still useful
	var funcs, types []string
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			name := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				name = recvName(d.Recv.List[0].Type) + "." + name
			}
			funcs = append(funcs, name)
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						types = append(types, ts.Name.Name)
					}
				}
			}
		}
	}
	var parts []string
	if len(types) > 0 {
		parts = append(parts, "type "+joinCapped(types))
	}
	if len(funcs) > 0 {
		parts = append(parts, "func "+joinCapped(funcs))
	}
	return strings.Join(parts, "; ")
}

func recvName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvName(t.X)
	case *ast.Ident:
		return t.Name
	default:
		return ""
	}
}

// joinCapped joins names with ", ", capping the count and noting the overflow.
func joinCapped(names []string) string {
	if len(names) > maxSymbolsPerFile {
		extra := len(names) - maxSymbolsPerFile
		names = append(names[:maxSymbolsPerFile:maxSymbolsPerFile], "…+"+itoa(extra))
	}
	return strings.Join(names, ", ")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
