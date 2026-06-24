package memory

import (
	"strings"
	"testing"
)

func TestAppendLoadDedupeClear(t *testing.T) {
	root := t.TempDir()

	if Load(root) != "" {
		t.Fatal("fresh project should have no memory")
	}
	if err := Append(root, "uses tabs, not spaces"); err != nil {
		t.Fatal(err)
	}
	if err := Append(root, "tests run with: go test ./..."); err != nil {
		t.Fatal(err)
	}
	got := Load(root)
	if !strings.Contains(got, "# Project memory") {
		t.Fatalf("first write should add a header:\n%s", got)
	}
	if !strings.Contains(got, "- uses tabs, not spaces") || !strings.Contains(got, "- tests run with: go test ./...") {
		t.Fatalf("both facts should be present:\n%s", got)
	}

	// Exact duplicate is skipped.
	_ = Append(root, "uses tabs, not spaces")
	if n := strings.Count(Load(root), "- uses tabs, not spaces"); n != 1 {
		t.Fatalf("duplicate fact should not repeat, count=%d", n)
	}

	// Newlines in a fact collapse to one bullet line.
	_ = Append(root, "multi\nline\nfact")
	if !strings.Contains(Load(root), "- multi line fact") {
		t.Fatalf("multiline fact should collapse:\n%s", Load(root))
	}

	if err := Clear(root); err != nil {
		t.Fatal(err)
	}
	if Load(root) != "" {
		t.Fatal("clear should empty memory")
	}
	// Clearing again (missing file) is not an error.
	if err := Clear(root); err != nil {
		t.Fatalf("clear on missing should be nil: %v", err)
	}
}

func TestSystemNote(t *testing.T) {
	if SystemNote("  ") != "" {
		t.Fatal("blank memory should produce no system note")
	}
	if !strings.Contains(SystemNote("- a fact"), "- a fact") {
		t.Fatal("system note should embed the memory")
	}
}
