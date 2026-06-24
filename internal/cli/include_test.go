package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandFileRefs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &session{dir: dir}

	// A real @file is inlined into the prompt; the typed line stays the body.
	prompt, notes, _ := s.expandFileRefs("explain @a.go please")
	if !strings.Contains(prompt, "func main()") || !strings.Contains(prompt, "--- a.go ---") {
		t.Fatalf("@file content + label should be inlined:\n%s", prompt)
	}
	if !strings.HasSuffix(strings.TrimRight(prompt, "\n"), "explain @a.go please") {
		t.Fatalf("the typed line should remain the prompt body:\n%s", prompt)
	}
	if len(notes) == 0 || !strings.Contains(strings.Join(notes, ""), "a.go") {
		t.Fatalf("an inclusion note should mention the file: %v", notes)
	}

	// An @token that is not a real file is left as literal text (no inlining).
	if p, _, _ := s.expandFileRefs("ping @nope.go"); p != "ping @nope.go" {
		t.Fatalf("a non-file @token must be left as text: %q", p)
	}
	// An email @host must not be slurped.
	if p, _, _ := s.expandFileRefs("mail me at a@b.com"); p != "mail me at a@b.com" {
		t.Fatalf("an email @host must not be treated as a file: %q", p)
	}

	// A path escaping the root is refused with a warning, never inlined.
	p4, n4, _ := s.expandFileRefs("read @../../etc/passwd")
	if strings.Contains(p4, "root:") || strings.Contains(p4, "--- ") {
		t.Fatalf("an out-of-root @path must not be inlined:\n%s", p4)
	}
	if len(n4) == 0 || !strings.Contains(strings.Join(n4, ""), "outside the project root") {
		t.Fatalf("an out-of-root @path should warn: %v", n4)
	}

	// An @image attaches as a vision image, NOT inlined as text.
	if err := os.WriteFile(filepath.Join(dir, "shot.png"), []byte("\x89PNG\x0d\x0a\x1a\x0aDATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	pImg, _, imgs := s.expandFileRefs("what's in @shot.png")
	if len(imgs) != 1 || imgs[0].MediaType != "image/png" {
		t.Fatalf("@image should attach one image/png, got %d", len(imgs))
	}
	if strings.Contains(pImg, "PNG") || strings.Contains(pImg, "--- shot.png") {
		t.Fatalf("image bytes must not be inlined as text:\n%s", pImg)
	}
}
