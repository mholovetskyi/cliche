package lineedit

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestMenuScrollKeepsSelectionVisible(t *testing.T) {
	cmds := make([]Command, 12) // more than maxMenuRows (8)
	for i := range cmds {
		cmds[i] = Command{Name: fmt.Sprintf("/cmd%02d", i)}
	}
	e := NewEditor(strings.NewReader(""), &bytes.Buffer{}, cmds, NewHistory(nil))
	e.menu.update("/") // open; all 12 match

	// Arrow down past the visible window (to the 10th item, index 9).
	for i := 0; i < 9; i++ {
		e.menu.down()
	}
	rows := e.menuRows()
	if len(rows) != maxMenuRows {
		t.Fatalf("window should be %d rows, got %d", maxMenuRows, len(rows))
	}
	want := cmds[9].Name // the selected command must be on screen
	onScreen := false
	for _, r := range rows {
		if strings.Contains(r, want) {
			onScreen = true
		}
	}
	if !onScreen {
		t.Fatalf("selected command %q must be visible after scrolling:\n%s", want, strings.Join(rows, "\n"))
	}
	// And completion targets exactly that visible selection.
	if got, ok := e.menu.completion(); !ok || got != want {
		t.Fatalf("completion = %q (ok=%v), want %q", got, ok, want)
	}
}
