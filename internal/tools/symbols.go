package tools

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// findSymbol locates where a symbol is DEFINED (op=definition, the default) or
// REFERENCED (op=references) across the project — the navigation half of code
// intelligence, without an external language server. Go files use the stdlib
// go/ast for precise definitions; other languages use declaration patterns. It is
// read-only and confined to the project root.
func (e OSExecutor) findSymbol(args map[string]string) Result {
	name := strings.TrimSpace(firstNonEmpty(args["name"], args["symbol"], args["query"]))
	if name == "" {
		return Result{Output: "find_symbol error: no symbol name given", Success: false}
	}
	op := strings.ToLower(strings.TrimSpace(firstNonEmpty(args["op"], "definition")))
	if op != "definition" && op != "references" {
		op = "definition"
	}
	root := e.Root
	if root == "" {
		root, _ = os.Getwd()
	}
	if p := strings.TrimSpace(firstNonEmpty(args["path"], args["dir"])); p != "" {
		if abs, err := e.resolve(p); err == nil {
			root = abs
		}
	}
	langFilter := strings.ToLower(strings.TrimSpace(args["language"]))

	const maxMatches = 500
	wordRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
	var matches []symMatch
	truncated := false

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		lang := langOf(filepath.Ext(path))
		if lang == "" || (langFilter != "" && lang != langFilter) {
			return nil
		}
		if len(matches) >= maxMatches {
			truncated = true
			return fs.SkipAll
		}
		rel := filepath.ToSlash(relOrSelf(root, path))
		if op == "references" {
			matches = append(matches, scanReferences(path, rel, wordRe)...)
			return nil
		}
		if lang == "go" {
			matches = append(matches, goDefinitions(path, rel, name)...)
		} else {
			matches = append(matches, regexDefinitions(path, rel, name, lang)...)
		}
		return nil
	})

	if len(matches) > maxMatches {
		matches = matches[:maxMatches]
		truncated = true
	}
	return Result{Output: renderSymbols(name, op, matches, truncated), Success: true}
}

type symMatch struct {
	File string
	Line int
	Col  int
	Sig  string
}

// langOf maps a file extension to a language key (empty = unsupported/skip).
func langOf(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "ts"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "js"
	case ".py":
		return "py"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	}
	return ""
}

// goDefinitions walks a Go file's AST for declarations named `name` (functions,
// methods as Type.Method, types, consts, vars), with precise positions.
func goDefinitions(path, rel, name string) []symMatch {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if f == nil {
		return nil
	}
	var out []symMatch
	add := func(pos token.Pos, sig string) {
		p := fset.Position(pos)
		out = append(out, symMatch{File: rel, Line: p.Line, Col: p.Column, Sig: clipLine(sig)})
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			full := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) > 0 {
				full = recvTypeName(d.Recv.List[0].Type) + "." + d.Name.Name
			}
			if d.Name.Name == name || full == name {
				add(d.Pos(), "func "+full)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name.Name == name {
						add(s.Pos(), "type "+s.Name.Name)
					}
				case *ast.ValueSpec:
					kw := "var"
					if d.Tok == token.CONST {
						kw = "const"
					}
					for _, id := range s.Names {
						if id.Name == name {
							add(id.Pos(), kw+" "+id.Name)
						}
					}
				}
			}
		}
	}
	return out
}

func recvTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr: // generic receiver, e.g. List[T]
		return recvTypeName(t.X)
	}
	return ""
}

// declRe builds a per-language declaration pattern for `name`.
func declRe(name, lang string) *regexp.Regexp {
	n := regexp.QuoteMeta(name)
	switch lang {
	case "ts", "js":
		return regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?(?:function|class|interface|type|enum|const|let|var)\s+` + n + `\b`)
	case "py":
		return regexp.MustCompile(`(?m)^\s*(?:async\s+)?(?:def|class)\s+` + n + `\b`)
	case "rust":
		return regexp.MustCompile(`(?m)^\s*(?:pub\s+)?(?:async\s+)?(?:fn|struct|enum|trait|type|const|static)\s+` + n + `\b`)
	case "java":
		return regexp.MustCompile(`(?m)\b(?:class|interface|enum|record)\s+` + n + `\b`)
	}
	return nil
}

func regexDefinitions(path, rel, name, lang string) []symMatch {
	re := declRe(name, lang)
	if re == nil {
		return nil
	}
	src, err := os.ReadFile(path)
	if err != nil || isProbablyBinary(src) {
		return nil
	}
	var out []symMatch
	lines := strings.Split(string(src), "\n")
	for i, ln := range lines {
		if loc := re.FindStringIndex(ln); loc != nil {
			out = append(out, symMatch{File: rel, Line: i + 1, Col: leadingNonSpace(ln) + 1, Sig: clipLine(ln)})
		}
	}
	return out
}

func scanReferences(path, rel string, wordRe *regexp.Regexp) []symMatch {
	src, err := os.ReadFile(path)
	if err != nil || isProbablyBinary(src) {
		return nil
	}
	var out []symMatch
	for i, ln := range strings.Split(string(src), "\n") {
		if loc := wordRe.FindStringIndex(ln); loc != nil {
			out = append(out, symMatch{File: rel, Line: i + 1, Col: loc[0] + 1, Sig: clipLine(ln)})
		}
	}
	return out
}

func renderSymbols(name, op string, matches []symMatch, truncated bool) string {
	if len(matches) == 0 {
		return fmt.Sprintf("find_symbol: no %s found for %q", op, name)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].File != matches[j].File {
			return matches[i].File < matches[j].File
		}
		return matches[i].Line < matches[j].Line
	})
	var b strings.Builder
	fmt.Fprintf(&b, "%d %s match(es) for %q:\n", len(matches), op, name)
	for _, m := range matches {
		fmt.Fprintf(&b, "%s:%d:%d  %s\n", m.File, m.Line, m.Col, m.Sig)
	}
	if truncated {
		b.WriteString("… (truncated at 500 matches; narrow with language= or a more specific name)\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func relOrSelf(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return rel
	}
	return path
}

func leadingNonSpace(s string) int {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return i
		}
	}
	return 0
}

// isProbablyBinary reports whether src looks non-textual (a NUL in the first 8KB).
func isProbablyBinary(src []byte) bool {
	n := len(src)
	if n > 8192 {
		n = 8192
	}
	for i := 0; i < n; i++ {
		if src[i] == 0 {
			return true
		}
	}
	return false
}
