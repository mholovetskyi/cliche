package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitFlow(t *testing.T) {
	if !Available() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	if IsRepo(dir) {
		t.Fatal("a fresh temp dir should not be a repo")
	}
	mustRun := func(args ...string) {
		if _, err := run(dir, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	mustRun("init")
	mustRun("config", "user.email", "t@example.com")
	mustRun("config", "user.name", "Tester")

	if !IsRepo(dir) {
		t.Fatal("should be a repo after init")
	}
	if HasChanges(dir) {
		t.Fatal("fresh repo should have no changes")
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !HasChanges(dir) {
		t.Fatal("should report changes after writing a file")
	}
	hash, stat, err := Commit(dir, "cliche: test")
	if err != nil || hash == "" {
		t.Fatalf("commit failed: hash=%q stat=%q err=%v", hash, stat, err)
	}
	if HasChanges(dir) {
		t.Fatal("work tree should be clean after commit")
	}
	// Nothing to commit now → ("","",nil).
	if h, _, err := Commit(dir, "noop"); err != nil || h != "" {
		t.Fatalf("commit with no changes should be a no-op, got h=%q err=%v", h, err)
	}

	if err := CreateBranch(dir, "cliche/feature"); err != nil {
		t.Fatal(err)
	}
	if CurrentBranch(dir) != "cliche/feature" {
		t.Fatalf("expected branch cliche/feature, got %q", CurrentBranch(dir))
	}
}
