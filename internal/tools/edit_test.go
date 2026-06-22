package tools

import (
	"errors"
	"testing"
)

func TestApplyEditExact(t *testing.T) {
	out, err := applyEdit("a\nB\nc\n", "B", "X", false)
	if err != nil {
		t.Fatal(err)
	}
	if out != "a\nX\nc\n" {
		t.Fatalf("got %q", out)
	}
}

func TestApplyEditEmptyOld(t *testing.T) {
	if _, err := applyEdit("x", "", "y", false); !errors.Is(err, ErrEmptyOld) {
		t.Fatalf("expected ErrEmptyOld, got %v", err)
	}
}

func TestApplyEditNotFound(t *testing.T) {
	if _, err := applyEdit("hello", "zzz", "y", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestApplyEditAmbiguous(t *testing.T) {
	if _, err := applyEdit("x\nx\n", "x", "y", false); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("expected ErrAmbiguous, got %v", err)
	}
}

func TestApplyEditReplaceAll(t *testing.T) {
	out, err := applyEdit("x\nx\n", "x", "y", true)
	if err != nil {
		t.Fatal(err)
	}
	if out != "y\ny\n" {
		t.Fatalf("got %q", out)
	}
}

func TestApplyEditExactPreservesIndent(t *testing.T) {
	// "return 1" is an exact substring (the tab precedes it), so the exact path
	// fires and indentation is preserved.
	out, err := applyEdit("func f() {\n\treturn 1\n}\n", "return 1", "return 2", false)
	if err != nil {
		t.Fatal(err)
	}
	if out != "func f() {\n\treturn 2\n}\n" {
		t.Fatalf("got %q", out)
	}
}

func TestApplyEditWhitespaceTolerant(t *testing.T) {
	// The file uses a TAB; the model's snippet uses SPACES, so it is not an
	// exact substring and the whitespace-tolerant line match must kick in.
	content := "func f() {\n\treturn 1\n}\n"
	out, err := applyEdit(content, "    return 1", "return 2", false)
	if err != nil {
		t.Fatalf("whitespace-tolerant match should succeed: %v", err)
	}
	if out != "func f() {\nreturn 2\n}\n" {
		t.Fatalf("got %q", out)
	}
}

func TestApplyEditReplaceAllNonOverlapping(t *testing.T) {
	// Indented lines force the whitespace-tolerant path; overlapping normalized
	// matches must NOT corrupt the file (regression for the backwards-rebuild).
	content := " x\n x\n x\n"
	out, err := applyEdit(content, "x\nx", "Y", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First non-overlapping block (lines 0-1) is replaced; the trailing line
	// remains. The result must be well-formed, not duplicated or truncated.
	if out != "Y\n x\n" {
		t.Fatalf("got %q", out)
	}
}

func TestApplyEditMultiLineBlock(t *testing.T) {
	content := "line0\n    if x {\n        do()\n    }\nline4\n"
	old := "if x {\ndo()\n}"
	out, err := applyEdit(content, old, "if y {\n    redo()\n}", false)
	if err != nil {
		t.Fatalf("multi-line block edit should succeed: %v", err)
	}
	want := "line0\nif y {\n    redo()\n}\nline4\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}
