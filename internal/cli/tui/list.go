// Package tui is Cliche's full-screen, mouse-driven terminal UI — the
// alternate-screen, pane-based mode that complements the inline line editor. It
// builds on the same zero-dependency foundation (keydec for input incl. SGR
// mouse, rawmode for the alt-screen/mouse escapes). The interactive driver can't
// be unit-tested headless, so the state (List) and the frame renderer are pure
// and covered by tests; the driver is a thin loop over them.
package tui

// List is the pure state of a scrollable single-selection list: a selection and
// a scroll offset within a viewport of Height rows. Every navigation keeps the
// selection inside the visible window.
type List struct {
	n      int
	Sel    int
	Off    int
	Height int
}

// NewList makes a list over n items.
func NewList(n int) *List { return &List{n: n, Height: 1} }

// SetHeight sets the viewport height (rows) and re-clamps the scroll window.
func (l *List) SetHeight(h int) {
	if h < 1 {
		h = 1
	}
	l.Height = h
	l.clamp()
}

// Up/Down/Page move the selection (clamped to the items), scrolling as needed.
func (l *List) Up()        { l.move(-1) }
func (l *List) Down()      { l.move(1) }
func (l *List) Page(d int) { l.move(d) }

func (l *List) move(d int) {
	l.Sel += d
	if l.Sel > l.n-1 { // clamp to the last item first…
		l.Sel = l.n - 1
	}
	if l.Sel < 0 { // …then floor at 0 (also fixes the empty-list n-1 == -1 case)
		l.Sel = 0
	}
	l.clamp()
}

// ClickRow selects the item shown at 0-based viewport row r (e.g. from a mouse
// click). Returns false when that row is empty (past the last item).
func (l *List) ClickRow(r int) bool {
	idx := l.Off + r
	if r < 0 || idx < 0 || idx >= l.n {
		return false
	}
	l.Sel = idx
	return true
}

// Window returns the [start, end) item indices currently visible.
func (l *List) Window() (int, int) {
	end := l.Off + l.Height
	if end > l.n {
		end = l.n
	}
	return l.Off, end
}

func (l *List) clamp() {
	if l.Sel < l.Off {
		l.Off = l.Sel
	}
	if l.Sel >= l.Off+l.Height {
		l.Off = l.Sel - l.Height + 1
	}
	if l.Off < 0 {
		l.Off = 0
	}
}
