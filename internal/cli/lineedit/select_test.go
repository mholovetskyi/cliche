package lineedit

import (
	"bytes"
	"strings"
	"testing"
)

func pickWith(t *testing.T, keys string, items []SelectItem) (int, bool) {
	t.Helper()
	e := NewEditor(strings.NewReader(keys), &bytes.Buffer{}, nil, NewHistory(nil))
	return e.Select("pick", items)
}

func TestSelectFilterThenEnter(t *testing.T) {
	items := []SelectItem{{Label: "apple"}, {Label: "banana"}, {Label: "cherry"}}
	// Type "ban" → only banana matches → Enter picks it (original index 1).
	if idx, ok := pickWith(t, "ban\r", items); !ok || idx != 1 {
		t.Fatalf("filter+enter = (%d,%v), want (1,true)", idx, ok)
	}
}

func TestSelectArrowThenEnter(t *testing.T) {
	items := []SelectItem{{Label: "one"}, {Label: "two"}, {Label: "three"}}
	// Down twice → index 2, Enter.
	if idx, ok := pickWith(t, "\x1b[B\x1b[B\r", items); !ok || idx != 2 {
		t.Fatalf("down,down,enter = (%d,%v), want (2,true)", idx, ok)
	}
}

func TestSelectEscCancels(t *testing.T) {
	items := []SelectItem{{Label: "x"}, {Label: "y"}}
	if idx, ok := pickWith(t, "\x1b", items); ok || idx != -1 {
		t.Fatalf("esc = (%d,%v), want (-1,false)", idx, ok)
	}
}

func TestSelectFilterRemapsIndex(t *testing.T) {
	// Filtering must return the index into the ORIGINAL slice, not the filtered view.
	items := []SelectItem{{Label: "alpha"}, {Label: "beta"}, {Label: "gamma"}, {Label: "delta"}}
	// "a" matches alpha(0), beta(1), gamma(2), delta(3) — all contain 'a'. Narrow to "elt" → delta(3).
	if idx, ok := pickWith(t, "elt\r", items); !ok || idx != 3 {
		t.Fatalf("narrowed filter = (%d,%v), want (3,true)", idx, ok)
	}
}
