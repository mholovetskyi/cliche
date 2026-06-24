package lineedit

import (
	"bytes"
	"strings"
	"testing"
)

func TestPhysicalRows(t *testing.T) {
	cases := []struct{ cells, cols, want int }{
		{0, 80, 1}, {1, 80, 1}, {80, 80, 1}, {81, 80, 2}, {160, 80, 2}, {161, 80, 3},
		{0, 0, 1}, // cols<1 guarded against divide-by-zero
	}
	for _, c := range cases {
		if got := physicalRows(c.cells, c.cols); got != c.want {
			t.Errorf("physicalRows(%d, %d) = %d, want %d", c.cells, c.cols, got, c.want)
		}
	}
}

func TestFooterShownWhenMenuClosedHiddenWhenOpen(t *testing.T) {
	var buf bytes.Buffer
	e := NewEditor(strings.NewReader(""), &buf, []Command{{Name: "/status"}}, NewHistory(nil))
	e.Footer = "FOOTER_HINT"
	e.prompt, e.promptW = "> ", 2

	// Menu closed → footer is drawn below the input.
	e.buf, e.cursor = []rune("hi"), 2
	e.render()
	if !strings.Contains(buf.String(), "FOOTER_HINT") {
		t.Fatalf("footer should render while the dropdown is closed:\n%q", buf.String())
	}

	// Dropdown open ("/") → it replaces the footer.
	buf.Reset()
	e.buf, e.cursor = []rune("/"), 1
	e.menu.update("/")
	e.render()
	out := buf.String()
	if strings.Contains(out, "FOOTER_HINT") {
		t.Fatalf("footer must yield to the dropdown when open:\n%q", out)
	}
	if !strings.Contains(out, "/status") {
		t.Fatalf("dropdown should list commands:\n%q", out)
	}
}

func TestRenderWrapsWideLine(t *testing.T) {
	var buf bytes.Buffer
	e := NewEditor(strings.NewReader(""), &buf, nil, NewHistory(nil))
	e.SetWidth(10)
	e.prompt, e.promptW = "> ", 2
	e.buf = []rune("abcdefghijklmnopqrstuvwxyz") // 26 + 2 prompt = 28 cells → 3 rows @ width 10
	e.cursor = len(e.buf)

	e.render()
	out := buf.String()
	if !strings.Contains(out, "\x1b[J") {
		t.Fatalf("render must erase with \\x1b[J (clears every wrapped row), got:\n%q", out)
	}
	if !strings.Contains(out, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("render must rewrite the full buffer:\n%q", out)
	}
	if e.prevCursorRow != 2 { // 28 cells / 10 cols = row 2
		t.Fatalf("prevCursorRow = %d, want 2", e.prevCursorRow)
	}

	// A subsequent render must climb back to the block's top row before clearing,
	// instead of the old single-row \r\x1b[K that corrupted wrapped lines.
	buf.Reset()
	e.render()
	if got := buf.String(); !strings.Contains(got, "\x1b[2A") {
		t.Fatalf("second render must move up 2 rows to the block top, got:\n%q", got)
	}
}
