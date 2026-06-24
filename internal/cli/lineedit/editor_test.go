package lineedit

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func runEditor(t *testing.T, script string, hist *History) (string, error) {
	t.Helper()
	var out bytes.Buffer
	ed := NewEditor(strings.NewReader(script), &out, testCmds(), hist)
	return ed.ReadLine("> ", 2)
}

func TestEditorBasicTyping(t *testing.T) {
	if line, err := runEditor(t, "abc\r", nil); err != nil || line != "abc" {
		t.Fatalf("'abc'+Enter = %q, %v", line, err)
	}
}

func TestEditorMutators(t *testing.T) {
	cases := []struct{ script, want string }{
		{"ac\x1b[Db\r", "abc"},         // Left then insert in the middle
		{"abX\x7f\r", "ab"},            // Backspace
		{"hello\x01\x0b\r", ""},        // Ctrl-A (home) + Ctrl-K (kill to end)
		{"hello\x15world\r", "world"},  // Ctrl-U (kill to start) then type
		{"foo bar\x17\r", "foo "},      // Ctrl-W (delete word left)
		{"ab\x1b[D\x05X\r", "abX"},     // Left then Ctrl-E (end) then type
		{"abc\x1b[H\x1b[CX\r", "aXbc"}, // Home, Right, insert
	}
	for _, c := range cases {
		if line, err := runEditor(t, c.script, nil); err != nil || line != c.want {
			t.Errorf("script %q = %q (%v), want %q", c.script, line, err, c.want)
		}
	}
}

func TestEditorTerminators(t *testing.T) {
	if _, err := runEditor(t, "\x03", nil); err != ErrInterrupted {
		t.Fatalf("Ctrl-C = %v, want ErrInterrupted", err)
	}
	if _, err := runEditor(t, "\x04", nil); err != io.EOF {
		t.Fatalf("Ctrl-D (empty) = %v, want io.EOF", err)
	}
	// Ctrl-D mid-line is forward-delete, not EOF.
	if line, err := runEditor(t, "ab\x1b[D\x04\r", nil); err != nil || line != "a" {
		t.Fatalf("Ctrl-D mid-line = %q, %v; want a", line, err)
	}
}

func TestEditorUTF8IsOneCell(t *testing.T) {
	if line, _ := runEditor(t, "héllo\r", nil); line != "héllo" {
		t.Fatalf("utf8 typing = %q", line)
	}
	// Backspace removes the whole multibyte rune.
	if line, _ := runEditor(t, "aé\x7f\r", nil); line != "a" {
		t.Fatalf("utf8 backspace = %q, want a", line)
	}
}

func TestEditorDropdownCompletion(t *testing.T) {
	cases := []struct{ script, want string }{
		{"/\t\r", "/status"},         // Tab completes the first match, Enter submits exact
		{"/di\t\r", "/diff"},         // Tab completes a unique prefix
		{"/di\r\r", "/diff"},         // Enter completes (menu open), Enter submits exact
		{"/c\x1b[B\t\r", "/context"}, // Down then Tab selects the 2nd match
		{"/cost \r", "/cost "},       // a space closes the menu; Enter submits as typed
	}
	for _, c := range cases {
		if line, err := runEditor(t, c.script, nil); err != nil || line != c.want {
			t.Errorf("script %q = %q (%v), want %q", c.script, line, err, c.want)
		}
	}
}

func TestEditorBackslashContinuation(t *testing.T) {
	// "first\" + Enter becomes a newline (keep editing); "second" + Enter submits.
	line, err := runEditor(t, "first\\\rsecond\r", nil)
	if err != nil || line != "first\nsecond" {
		t.Fatalf("backslash continuation = %q, %v; want %q", line, err, "first\nsecond")
	}
}

func TestEditorMultiLinePaste(t *testing.T) {
	// Type "a", paste a two-line block, then Enter → the buffer keeps the newline.
	line, err := runEditor(t, "a\x1b[200~x\ny\x1b[201~\r", nil)
	if err != nil || line != "ax\ny" {
		t.Fatalf("multi-line paste = %q, %v; want %q", line, err, "ax\ny")
	}
}

func TestEditorShiftTabCyclesMode(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	ed := NewEditor(strings.NewReader("\x1b[Z\r"), &out, testCmds(), nil)
	ed.CycleMode = func() (string, int) { calls++; return "X ", 2 }
	if line, err := ed.ReadLine("> ", 2); err != nil || line != "" {
		t.Fatalf("ReadLine = %q, %v", line, err)
	}
	if calls != 1 {
		t.Fatalf("Shift-Tab should invoke CycleMode once, got %d", calls)
	}
}

func TestEditorHistory(t *testing.T) {
	h := NewHistory([]string{"prev-cmd"})
	if line, _ := runEditor(t, "\x1b[A\r", h); line != "prev-cmd" {
		t.Fatalf("Up then Enter = %q, want prev-cmd", line)
	}
	// A submitted line is added to history.
	h2 := NewHistory(nil)
	runEditor(t, "first\r", h2)
	if line, _ := runEditor(t, "\x1b[A\r", h2); line != "first" {
		t.Fatalf("history should record submitted lines, Up = %q want first", line)
	}
}
