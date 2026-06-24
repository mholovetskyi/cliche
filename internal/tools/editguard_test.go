package tools

import (
	"strings"
	"testing"
)

func TestGuardCollateralDeletion(t *testing.T) {
	old := "package x\n\nfunc A() {}\n\nfunc B() {}\n"

	// Collateral: the edit targets A but drops B, and old_string never named B.
	err := guardCollateralDeletion("x.go", old, "package x\n\nfunc A2() {}\n", "func A() {}")
	if err == nil || !strings.Contains(err.Error(), "B") {
		t.Fatalf("should reject dropping B (unreferenced), got: %v", err)
	}

	// Intentional: old_string names B, so removing it is allowed.
	if err := guardCollateralDeletion("x.go", old, "package x\n\nfunc A() {}\n", "func B() {}"); err != nil {
		t.Fatalf("intentional removal (B in old_string) should pass: %v", err)
	}

	// Benign: a normal in-place edit removes nothing.
	if err := guardCollateralDeletion("x.go", old, "package x\n\nfunc A() { return }\n\nfunc B() {}\n", "func A() {}"); err != nil {
		t.Fatalf("non-deleting edit should pass: %v", err)
	}

	// Non-Go files are out of scope.
	if err := guardCollateralDeletion("x.txt", "line one\nline two\n", "line one\n", ""); err != nil {
		t.Fatalf("non-Go should be skipped: %v", err)
	}
}

func TestGoTopLevelDecls(t *testing.T) {
	got := goTopLevelDecls("package p\n\ntype T struct{}\n\nconst C = 1\n\nvar V int\n\nfunc F() {}\n")
	for _, name := range []string{"T", "C", "V", "F"} {
		if !got[name] {
			t.Errorf("missing top-level decl %q in %v", name, got)
		}
	}
}
