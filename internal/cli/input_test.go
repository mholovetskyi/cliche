package cli

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/style"
)

func TestReadInputMultiline(t *testing.T) {
	// Backslash continuation joins lines with a newline; the next read is normal.
	s := &session{r: bufio.NewReader(strings.NewReader("first\\\nsecond\nrest\n")), out: &bytes.Buffer{}}
	if got, _ := s.readInput(); got != "first\nsecond" {
		t.Fatalf("backslash continuation = %q, want %q", got, "first\nsecond")
	}
	if got, _ := s.readInput(); got != "rest" {
		t.Fatalf("the following line should read normally = %q, want %q", got, "rest")
	}

	// A slash command is always single-line, even with a trailing backslash.
	s2 := &session{r: bufio.NewReader(strings.NewReader("/cost\n")), out: &bytes.Buffer{}}
	if got, _ := s2.readInput(); got != "/cost" {
		t.Fatalf("slash command should stay single-line: %q", got)
	}

	// EOF at an empty prompt surfaces an error so the loop can exit.
	s3 := &session{r: bufio.NewReader(strings.NewReader("")), out: &bytes.Buffer{}}
	if _, err := s3.readInput(); err == nil {
		t.Fatal("EOF at an empty prompt should return an error")
	}
}

func TestReadLineInteractiveFallsBackToCooked(t *testing.T) {
	old := style.Enabled
	style.Enabled = false // not a styled TTY → the raw editor is skipped
	defer func() { style.Enabled = old }()

	s := &session{r: bufio.NewReader(strings.NewReader("hello world\n")), out: &bytes.Buffer{}}
	got, err := s.readLineInteractive()
	if err != nil || got != "hello world" {
		t.Fatalf("interactive read should fall back to cooked input = %q, %v; want 'hello world'", got, err)
	}
}
