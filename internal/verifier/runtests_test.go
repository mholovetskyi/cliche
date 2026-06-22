package verifier

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunTestsPassAndFail(t *testing.T) {
	pass := RunTests(context.Background(), t.TempDir(), "exit 0")
	if !pass.Ran || !pass.Passed {
		t.Fatalf("expected pass, got %+v", pass)
	}
	fail := RunTests(context.Background(), t.TempDir(), "exit 1")
	if !fail.Ran || fail.Passed {
		t.Fatalf("expected fail, got %+v", fail)
	}
}

func TestDiscoverTestCommandGo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd, ok := DiscoverTestCommand(dir)
	if !ok || cmd != "go test ./..." {
		t.Fatalf("expected go test, got %q (ok=%v)", cmd, ok)
	}
}

func TestDiscoverTestCommandFromAgents(t *testing.T) {
	dir := t.TempDir()
	agents := "# AGENTS\n\n## verify\n\ntest: make check\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(agents), 0o644); err != nil {
		t.Fatal(err)
	}
	// Even with a go.mod present, the AGENTS.md verify section wins.
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644)
	cmd, ok := DiscoverTestCommand(dir)
	if !ok || cmd != "make check" {
		t.Fatalf("expected 'make check' from AGENTS.md, got %q", cmd)
	}
}

func TestDiscoverTestCommandNone(t *testing.T) {
	if _, ok := DiscoverTestCommand(t.TempDir()); ok {
		t.Fatal("expected no command for an empty dir")
	}
}

func TestVerifyClaim(t *testing.T) {
	clean := "--- a/x.go\n+++ b/x.go\n-x := 1\n+x := 2\n"
	hack := "--- a/x_test.go\n+++ b/x_test.go\n-func TestPay(t *testing.T) {\n-}\n"

	// Reward-hack pattern takes priority over a passing re-run.
	if v := VerifyClaim(hack, true, TestResult{Ran: true, Passed: true}); v.Status != StatusFlagged {
		t.Fatalf("hack diff should be flagged, got %s", v.Status)
	}
	// Clean diff + passing tests => verified (only reachable via re-run).
	if v := VerifyClaim(clean, true, TestResult{Ran: true, Passed: true}); v.Status != StatusVerified {
		t.Fatalf("clean diff + passing tests should be verified, got %s", v.Status)
	}
	// Clean diff + failing tests + claim => flagged with a false-claim finding.
	v := VerifyClaim(clean, true, TestResult{Ran: true, Passed: false, Command: "go test ./..."})
	if v.Status != StatusFlagged {
		t.Fatalf("failing tests should be flagged, got %s", v.Status)
	}
	if len(v.Findings) == 0 || v.Findings[0].Rule != "false_pass_claim" {
		t.Fatalf("expected false_pass_claim finding, got %+v", v.Findings)
	}
	// No re-run => unverified, never verified.
	if v := VerifyClaim(clean, false, TestResult{Ran: false}); v.Status != StatusUnverified {
		t.Fatalf("no re-run should be unverified, got %s", v.Status)
	}
}
