package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scaffold writes a small project tree under a temp root and returns the root.
func scaffold(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"main.go":                    "package main\n\nfunc main() { hello() }\n",
		"internal/util/util.go":      "package util\n\nfunc hello() string { return \"hi\" }\n",
		"internal/util/util_test.go": "package util\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) {}\n",
		"docs/readme.md":             "# Title\n\nhello world\n",
		".git/config":                "[core]\n",
		"node_modules/dep/index.js":  "function hello() {}\n",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A binary file that must be skipped by search.
	if err := os.WriteFile(filepath.Join(root, "bin.dat"), []byte{'h', 'e', 0, 'l', 'l', 'o'}, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestSearchFilesFindsMatchesAndSkipsNoise(t *testing.T) {
	root := scaffold(t)
	e := OSExecutor{Root: root}
	r := e.Execute(context.Background(), "search_files", map[string]string{"pattern": "hello"})
	if !r.Success {
		t.Fatalf("search should succeed: %s", r.Output)
	}
	// Finds real source matches.
	if !strings.Contains(r.Output, "main.go:") || !strings.Contains(r.Output, "util.go:") {
		t.Fatalf("expected matches in source files, got:\n%s", r.Output)
	}
	// Skips .git, node_modules, and binary files.
	for _, noise := range []string{".git", "node_modules", "bin.dat"} {
		if strings.Contains(r.Output, noise) {
			t.Fatalf("search must skip %q, got:\n%s", noise, r.Output)
		}
	}
}

func TestSearchFilesGlobFilter(t *testing.T) {
	root := scaffold(t)
	e := OSExecutor{Root: root}
	r := e.Execute(context.Background(), "search_files", map[string]string{"pattern": "hello", "glob": "*.md"})
	if !r.Success {
		t.Fatalf("search should succeed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "readme.md") {
		t.Fatalf("glob *.md should match the doc, got:\n%s", r.Output)
	}
	if strings.Contains(r.Output, ".go:") {
		t.Fatalf("glob *.md must exclude .go files, got:\n%s", r.Output)
	}
}

func TestSearchFilesIgnoreCaseAndNoMatch(t *testing.T) {
	root := scaffold(t)
	e := OSExecutor{Root: root}
	if r := e.Execute(context.Background(), "search_files", map[string]string{"pattern": "HELLO"}); strings.Contains(r.Output, "main.go") {
		t.Fatalf("case-sensitive search should not match HELLO, got:\n%s", r.Output)
	}
	r := e.Execute(context.Background(), "search_files", map[string]string{"pattern": "HELLO", "ignore_case": "true"})
	if !r.Success || !strings.Contains(r.Output, "main.go") {
		t.Fatalf("ignore_case should match, got:\n%s", r.Output)
	}
	if r := e.Execute(context.Background(), "search_files", map[string]string{"pattern": "nonexistent_token"}); !r.Success || !strings.Contains(r.Output, "no matches") {
		t.Fatalf("absent token should report no matches, got:\n%s", r.Output)
	}
}

func TestSearchFilesInvalidRegex(t *testing.T) {
	root := scaffold(t)
	e := OSExecutor{Root: root}
	if r := e.Execute(context.Background(), "search_files", map[string]string{"pattern": "("}); r.Success {
		t.Fatalf("invalid regex must fail, got success: %s", r.Output)
	}
}

func TestFindFilesGlob(t *testing.T) {
	root := scaffold(t)
	e := OSExecutor{Root: root}

	// Bare *.go matches by base name at any depth.
	r := e.Execute(context.Background(), "find_files", map[string]string{"pattern": "*.go"})
	if !r.Success {
		t.Fatalf("find should succeed: %s", r.Output)
	}
	for _, want := range []string{"main.go", "internal/util/util.go", "internal/util/util_test.go"} {
		if !strings.Contains(r.Output, want) {
			t.Fatalf("expected %q in results, got:\n%s", want, r.Output)
		}
	}

	// **-aware path glob.
	r = e.Execute(context.Background(), "find_files", map[string]string{"pattern": "internal/**/*_test.go"})
	if !strings.Contains(r.Output, "internal/util/util_test.go") {
		t.Fatalf("**-glob should match the test file, got:\n%s", r.Output)
	}
	if strings.Contains(r.Output, "main.go") {
		t.Fatalf("path glob must not match main.go, got:\n%s", r.Output)
	}

	// node_modules is skipped.
	r = e.Execute(context.Background(), "find_files", map[string]string{"pattern": "*.js"})
	if strings.Contains(r.Output, "node_modules") {
		t.Fatalf("find must skip node_modules, got:\n%s", r.Output)
	}
}

func TestListFiles(t *testing.T) {
	root := scaffold(t)
	e := OSExecutor{Root: root}
	r := e.Execute(context.Background(), "list_files", map[string]string{})
	if !r.Success {
		t.Fatalf("list should succeed: %s", r.Output)
	}
	if !strings.Contains(r.Output, "main.go") || !strings.Contains(r.Output, "internal/") {
		t.Fatalf("listing should show files and dir (with trailing slash), got:\n%s", r.Output)
	}

	r = e.Execute(context.Background(), "list_files", map[string]string{"path": "internal/util"})
	if !strings.Contains(r.Output, "util.go") || strings.Contains(r.Output, "main.go") {
		t.Fatalf("scoped listing should show only that dir, got:\n%s", r.Output)
	}
}

func TestSearchConfinement(t *testing.T) {
	root := scaffold(t)
	e := OSExecutor{Root: root}
	for _, tool := range []string{"search_files", "find_files", "list_files"} {
		args := map[string]string{"pattern": "x", "path": ".."}
		if tool == "list_files" {
			args = map[string]string{"path": ".."}
		}
		if r := e.Execute(context.Background(), tool, args); r.Success {
			t.Fatalf("%s must refuse a path outside the root, got: %s", tool, r.Output)
		}
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pat, name string
		want      bool
	}{
		{"**/*.go", "a/b/c.go", true},
		{"**/*.go", "c.go", true},
		{"internal/**/*.go", "internal/x/y.go", true},
		{"internal/**/*.go", "cmd/x.go", false},
		{"a/*/c", "a/b/c", true},
		{"a/*/c", "a/b/d/c", false},
		{"**", "anything/at/all", true},
	}
	for _, c := range cases {
		if got := globMatch(c.pat, c.name); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.pat, c.name, got, c.want)
		}
	}
}
