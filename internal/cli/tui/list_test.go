package tui

import "testing"

func TestListScrollKeepsSelectionVisible(t *testing.T) {
	l := NewList(20)
	l.SetHeight(5)
	// Move down past the viewport — the window must scroll to keep Sel visible.
	for i := 0; i < 7; i++ {
		l.Down()
	}
	if l.Sel != 7 {
		t.Fatalf("Sel = %d, want 7", l.Sel)
	}
	start, end := l.Window()
	if l.Sel < start || l.Sel >= end {
		t.Fatalf("selection %d outside window [%d,%d)", l.Sel, start, end)
	}
	if end-start != 5 {
		t.Fatalf("window height = %d, want 5", end-start)
	}
	// Back up to the top.
	for i := 0; i < 10; i++ {
		l.Up()
	}
	if l.Sel != 0 || l.Off != 0 {
		t.Fatalf("after Up*: Sel=%d Off=%d, want 0,0", l.Sel, l.Off)
	}
}

func TestListClickRow(t *testing.T) {
	l := NewList(10)
	l.SetHeight(4)
	for i := 0; i < 5; i++ { // scroll so Off > 0
		l.Down()
	}
	start, _ := l.Window()
	if !l.ClickRow(1) || l.Sel != start+1 {
		t.Fatalf("ClickRow(1) should select item %d, got Sel=%d", start+1, l.Sel)
	}
	// A click past the last item is a no-op.
	if l.ClickRow(99) {
		t.Fatal("click past the items should return false")
	}
}

func TestListEmpty(t *testing.T) {
	l := NewList(0)
	l.SetHeight(5)
	l.Down()
	l.Up()
	if l.Sel != 0 {
		t.Fatalf("empty list Sel = %d, want 0", l.Sel)
	}
	if l.ClickRow(0) {
		t.Fatal("click on empty list should be false")
	}
}
