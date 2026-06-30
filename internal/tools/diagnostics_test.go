package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scaffoldGoMod(t *testing.T, mainSrc string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module diagtest\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(mainSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func runDiag(t *testing.T, root string) Result {
	t.Helper()
	e := OSExecutor{Root: root, Policy: Policy{Yolo: true}}
	return e.diagnostics(context.Background(), nil)
}

func TestDiagnosticsCleanGo(t *testing.T) {
	root := scaffoldGoMod(t, "package main\n\nfunc main() {}\n")
	res := runDiag(t, root)
	if !res.Success {
		t.Fatalf("clean module should pass, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "0 error") {
		t.Fatalf("expected 0 errors, got: %s", res.Output)
	}
}

func TestDiagnosticsBrokenGoParsed(t *testing.T) {
	// A real compile error — locks the corrected Windows/Unix Go regex against
	// actual `go build` output (".\main.go:N:M: undefined: undeclared").
	root := scaffoldGoMod(t, "package main\n\nfunc main() {\n\tundeclared()\n}\n")
	res := runDiag(t, root)
	if res.Success {
		t.Fatalf("a compile error must fail the check, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "main.go") || !strings.Contains(res.Output, "undefined") {
		t.Fatalf("compile error not parsed into a structured diagnostic: %s", res.Output)
	}
	if !strings.Contains(res.Output, ": error:") {
		t.Fatalf("build error should be severity 'error': %s", res.Output)
	}
}

func TestDiagnosticsVetFinding(t *testing.T) {
	// Compiles, but `go vet` flags the Printf arg mismatch — proves vet runs and is
	// parsed independent of the build's exit code.
	root := scaffoldGoMod(t, "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Printf(\"%d %d\", 1)\n}\n")
	res := runDiag(t, root)
	if !strings.Contains(res.Output, "Printf") && !strings.Contains(res.Output, "arg") {
		t.Fatalf("expected a go vet Printf finding, got: %s", res.Output)
	}
}

func TestDiagnosticsNoProject(t *testing.T) {
	e := OSExecutor{Root: t.TempDir(), Policy: Policy{Yolo: true}}
	res := e.diagnostics(context.Background(), nil)
	if !res.Success || !strings.Contains(res.Output, "no supported project") {
		t.Fatalf("empty dir should be a graceful pass, got: %q (ok=%v)", res.Output, res.Success)
	}
}

func TestDiagnosticsPermissionGated(t *testing.T) {
	root := scaffoldGoMod(t, "package main\n\nfunc main() {}\n")
	e := OSExecutor{Root: root} // no Yolo, no AllowRun, no approver → denied
	if res := e.diagnostics(context.Background(), nil); res.Success {
		t.Fatal("diagnostics must be permission-gated like run_command")
	}
}
