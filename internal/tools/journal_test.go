package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestJournalRecordsAndDiffs(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "a.txt")
	if err := os.WriteFile(existing, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	j := NewEditJournal(root)
	e := OSExecutor{Root: root, Policy: Policy{Yolo: true}, Journal: j}

	// Edit an existing file and create a new one.
	if r := e.Execute(context.Background(), "edit_file", map[string]string{"file": "a.txt", "old_string": "two", "new_string": "TWO"}); !r.Success {
		t.Fatalf("edit failed: %s", r.Output)
	}
	if r := e.Execute(context.Background(), "write_file", map[string]string{"file": "b.txt", "content": "new file\n"}); !r.Success {
		t.Fatalf("write failed: %s", r.Output)
	}

	changes := j.Changes()
	if len(changes) != 2 {
		t.Fatalf("expected 2 changed files, got %d: %+v", len(changes), changes)
	}
	byPath := map[string]FileChange{}
	for _, c := range changes {
		byPath[c.Path] = c
	}
	a := byPath["a.txt"]
	if a.WasNew || a.Before != "one\ntwo\n" || a.After != "one\nTWO\n" {
		t.Fatalf("a.txt change wrong: %+v", a)
	}
	b := byPath["b.txt"]
	if !b.WasNew || b.Before != "" || b.After != "new file\n" {
		t.Fatalf("b.txt change wrong: %+v", b)
	}
}

func TestJournalNetNoOpOmitted(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "a.txt")
	if err := os.WriteFile(f, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	j := NewEditJournal(root)
	e := OSExecutor{Root: root, Policy: Policy{Yolo: true}, Journal: j}
	// Change then change back: net no-op.
	e.Execute(context.Background(), "edit_file", map[string]string{"file": "a.txt", "old_string": "hello", "new_string": "world"})
	e.Execute(context.Background(), "edit_file", map[string]string{"file": "a.txt", "old_string": "world", "new_string": "hello"})
	if c := j.Changes(); len(c) != 0 {
		t.Fatalf("net no-op should not appear in changes, got %+v", c)
	}
}

func TestJournalUndoRestoresAndUncreates(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "a.txt")
	if err := os.WriteFile(existing, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	j := NewEditJournal(root)
	e := OSExecutor{Root: root, Policy: Policy{Yolo: true}, Journal: j}

	e.Execute(context.Background(), "edit_file", map[string]string{"file": "a.txt", "old_string": "original", "new_string": "changed"})
	e.Execute(context.Background(), "write_file", map[string]string{"file": "b.txt", "content": "created\n"})

	// Undo the most recent op (the new file) → it should be removed.
	path, did, err := j.Undo()
	if err != nil || !did || path != "b.txt" {
		t.Fatalf("undo of new file: path=%q did=%v err=%v", path, did, err)
	}
	if _, err := os.Stat(filepath.Join(root, "b.txt")); !os.IsNotExist(err) {
		t.Fatal("undo should have removed the created file")
	}

	// Undo the edit → original content restored.
	path, did, err = j.Undo()
	if err != nil || !did || path != "a.txt" {
		t.Fatalf("undo of edit: path=%q did=%v err=%v", path, did, err)
	}
	got, _ := os.ReadFile(existing)
	if string(got) != "original\n" {
		t.Fatalf("undo should restore original content, got %q", got)
	}

	// Nothing left to undo.
	if _, did, _ := j.Undo(); did {
		t.Fatal("undo on empty journal should report nothing done")
	}
}

// TestJournalRelPathDefaultRoot reproduces the production wiring where the
// journal root is "." but the executor records absolute paths: display paths
// must still come out clean and relative, not as absolute leaks.
func TestJournalRelPathDefaultRoot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if real, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = real
	}
	j := NewEditJournal(".")
	abs := filepath.Join(cwd, "sub", "x.txt")
	if got := j.relPath(abs); got != "sub/x.txt" {
		t.Fatalf("relPath(%q) with root %q = %q, want \"sub/x.txt\"", abs, ".", got)
	}
}

func TestNilJournalIsNoOp(t *testing.T) {
	// An executor with no journal must work exactly as before.
	root := t.TempDir()
	e := OSExecutor{Root: root, Policy: Policy{Yolo: true}}
	if r := e.Execute(context.Background(), "write_file", map[string]string{"file": "x.txt", "content": "hi"}); !r.Success {
		t.Fatalf("write without a journal should still succeed: %s", r.Output)
	}
	var j *EditJournal
	if c := j.Changes(); c != nil {
		t.Fatal("nil journal Changes should be nil")
	}
	if _, did, _ := j.Undo(); did {
		t.Fatal("nil journal Undo should be a no-op")
	}
}
