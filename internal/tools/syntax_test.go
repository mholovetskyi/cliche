package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSyntaxGo(t *testing.T) {
	if err := validateSyntax("x.go", "package x\n\nfunc F() int { return 1 }\n"); err != nil {
		t.Fatalf("valid Go should pass: %v", err)
	}
	if err := validateSyntax("x.go", "package x\nfunc F( {"); err == nil {
		t.Fatal("broken Go should fail validation")
	}
	if err := validateSyntax("notes.txt", "func F( {"); err != nil {
		t.Fatalf("non-Go files should pass through: %v", err)
	}
}

func TestValidateSyntaxJSON(t *testing.T) {
	if err := validateSyntax("c.json", `{"a":1,"b":[2,3]}`); err != nil {
		t.Fatalf("valid JSON should pass: %v", err)
	}
	if err := validateSyntax("c.json", `{"a":1,}`); err == nil {
		t.Fatal("invalid JSON should fail validation")
	}
}

func TestEditFileRejectsBrokenGoAndLeavesFileIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	original := "package x\n\nfunc F() int {\n\treturn 1\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	e := OSExecutor{Policy: Policy{AllowWrite: true}}

	// An edit that produces clearly broken Go must be rejected and NOT written.
	broken := e.Execute(context.Background(), "edit_file", map[string]string{
		"file": path, "old_string": "func F() int {", "new_string": "func F() int {{{",
	})
	if broken.Success {
		t.Fatal("edit producing invalid Go should fail")
	}
	after, _ := os.ReadFile(path)
	if string(after) != original {
		t.Fatalf("file must be unchanged after a rejected edit, got:\n%s", after)
	}

	// A valid edit succeeds and is written.
	ok := e.Execute(context.Background(), "edit_file", map[string]string{
		"file": path, "old_string": "return 1", "new_string": "return 2",
	})
	if !ok.Success {
		t.Fatalf("valid edit should succeed: %s", ok.Output)
	}
	after, _ = os.ReadFile(path)
	if !strings.Contains(string(after), "return 2") {
		t.Fatalf("valid edit not applied:\n%s", after)
	}
}
