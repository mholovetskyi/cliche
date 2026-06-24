package tools

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
)

// goTopLevelDecls returns the set of top-level declaration names in Go source —
// functions/methods, types, and top-level vars/consts. A parse failure yields
// nil (validateSyntax reports syntax problems separately).
func goTopLevelDecls(content string) map[string]bool {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", content, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	names := map[string]bool{}
	for _, d := range f.Decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			names[decl.Name.Name] = true
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					names[s.Name.Name] = true
				case *ast.ValueSpec:
					for _, n := range s.Names {
						names[n.Name] = true
					}
				}
			}
		}
	}
	return names
}

// guardCollateralDeletion rejects a Go edit that removes a top-level declaration
// the edit never referenced — the signature of a botched edit that clobbers code
// the model wasn't even looking at. Observed live: a stray edit silently dropped
// a function and the result still PARSED, so the syntax check passed and the loss
// went unnoticed until tests failed. Deleting a declaration on purpose still
// works — name it in old_string. Conservative by design: a name appearing
// anywhere in old_string counts as referenced, so legitimate edits pass freely.
func guardCollateralDeletion(path, oldContent, newContent, oldStr string) error {
	if strings.ToLower(filepath.Ext(path)) != ".go" {
		return nil
	}
	before := goTopLevelDecls(oldContent)
	if before == nil {
		return nil // old file didn't parse — nothing to protect
	}
	after := goTopLevelDecls(newContent)
	var lost []string
	for name := range before {
		if !after[name] && !strings.Contains(oldStr, name) {
			lost = append(lost, name)
		}
	}
	if len(lost) == 0 {
		return nil
	}
	sort.Strings(lost)
	return fmt.Errorf("edit would remove top-level declaration(s) [%s] that your old_string never referenced — this looks like collateral deletion, not the change you described. To remove them on purpose, include them in old_string; otherwise narrow your edit so it touches only the intended code",
		strings.Join(lost, ", "))
}
