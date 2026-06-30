package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scaffoldSymbols(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"util.go":           "package x\n\ntype Widget struct{}\n\nfunc Hello(name string) string {\n\treturn name\n}\n",
		"main.go":           "package x\n\nfunc use() {\n\t_ = Hello(\"world\")\n}\n",
		"app.ts":            "export function Hello(): void {}\nexport const Widget = 1\n",
		".git/config":       "[core]\n",                     // must be skipped
		"node_modules/m.go": "package m\nfunc Hello() {}\n", // must be skipped
	}
	for rel, src := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestFindSymbolGoDefinition(t *testing.T) {
	e := OSExecutor{Root: scaffoldSymbols(t)}
	res := e.findSymbol(map[string]string{"name": "Hello", "op": "definition", "language": "go"})
	if !res.Success {
		t.Fatalf("expected success: %s", res.Output)
	}
	if !strings.Contains(res.Output, "util.go:5") {
		t.Fatalf("Go definition should point at util.go:5, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "func Hello") {
		t.Fatalf("expected the func signature, got: %s", res.Output)
	}
	// .git and node_modules must be skipped.
	if strings.Contains(res.Output, "node_modules") || strings.Contains(res.Output, ".git") {
		t.Fatalf("skipDirs not respected: %s", res.Output)
	}
}

func TestFindSymbolType(t *testing.T) {
	e := OSExecutor{Root: scaffoldSymbols(t)}
	res := e.findSymbol(map[string]string{"name": "Widget", "op": "definition", "language": "go"})
	if !strings.Contains(res.Output, "util.go:3") || !strings.Contains(res.Output, "type Widget") {
		t.Fatalf("type definition not found: %s", res.Output)
	}
}

func TestFindSymbolReferences(t *testing.T) {
	e := OSExecutor{Root: scaffoldSymbols(t)}
	res := e.findSymbol(map[string]string{"name": "Hello", "op": "references", "language": "go"})
	if !strings.Contains(res.Output, "main.go") {
		t.Fatalf("references should include the call site in main.go: %s", res.Output)
	}
}

func TestFindSymbolGenericLang(t *testing.T) {
	e := OSExecutor{Root: scaffoldSymbols(t)}
	res := e.findSymbol(map[string]string{"name": "Hello", "op": "definition", "language": "ts"})
	if !strings.Contains(res.Output, "app.ts:1") {
		t.Fatalf("TS declaration not found: %s", res.Output)
	}
}

func TestFindSymbolMissingName(t *testing.T) {
	e := OSExecutor{Root: t.TempDir()}
	if res := e.findSymbol(map[string]string{}); res.Success {
		t.Fatal("missing name should fail")
	}
}
