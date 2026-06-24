package lineedit

import "testing"

func testCmds() []Command {
	return []Command{
		{Name: "/status", Desc: "status"},
		{Name: "/cost", Desc: "cost"},
		{Name: "/context", Desc: "context"},
		{Name: "/clear", Desc: "clear"},
		{Name: "/commit", Args: "[msg]", Desc: "commit"},
		{Name: "/diff", Desc: "diff"},
	}
}

func filteredNames(m *slashMenu) []string {
	out := make([]string, len(m.filtered))
	for i, c := range m.filtered {
		out[i] = c.Name
	}
	return out
}

func eqStr(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMenuOpenAndFilter(t *testing.T) {
	m := newSlashMenu(testCmds())

	m.update("/")
	if !m.open || len(m.filtered) != 6 {
		t.Fatalf("'/' should open with all commands: open=%v n=%d", m.open, len(m.filtered))
	}
	m.update("/c")
	if got := filteredNames(m); !eqStr(got, []string{"/cost", "/context", "/clear", "/commit"}) {
		t.Fatalf("'/c' filtered = %v", got)
	}
	m.update("/co")
	if got := filteredNames(m); !eqStr(got, []string{"/cost", "/context", "/commit"}) {
		t.Fatalf("'/co' filtered = %v", got)
	}
	m.update("/cost x") // a space closes the menu
	if m.open {
		t.Fatal("a space after the command should close the menu")
	}
	m.update("hello") // non-slash closes too
	if m.open {
		t.Fatal("a non-slash buffer should keep the menu closed")
	}
}

func TestMenuNavWrapAndCompletion(t *testing.T) {
	m := newSlashMenu(testCmds())
	m.update("/co") // [/cost, /context, /commit]
	if m.sel != 0 {
		t.Fatalf("sel should start at 0, got %d", m.sel)
	}
	m.down()
	if m.sel != 1 {
		t.Fatalf("down -> %d, want 1", m.sel)
	}
	m.up()
	m.up() // wrap from 0 to last
	if m.sel != 2 {
		t.Fatalf("up wrap -> %d, want 2", m.sel)
	}
	// /commit takes args → completion carries a trailing space.
	if c, ok := m.completion(); !ok || c != "/commit " {
		t.Fatalf("completion = %q, %v; want '/commit ' true", c, ok)
	}
	// A no-args command completes to just its name.
	m.update("/di")
	if c, ok := m.completion(); !ok || c != "/diff" {
		t.Fatalf("completion = %q, %v; want '/diff' true", c, ok)
	}
}
