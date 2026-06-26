package lineedit

import "testing"

func TestFuzzyMatchBasics(t *testing.T) {
	cases := []struct {
		pat, s string
		want   bool
	}{
		{"/mdl", "/models", true},    // skip-a-letter still matches
		{"/mo", "/models", true},     // prefix
		{"/status", "/status", true}, // exact
		{"", "/anything", true},      // empty pattern matches all
		{"/xyz", "/models", false},   // no subsequence
		{"/MO", "/models", true},     // case-insensitive
	}
	for _, c := range cases {
		if _, _, ok := fuzzyMatch(c.pat, c.s); ok != c.want {
			t.Errorf("fuzzyMatch(%q,%q) ok=%v, want %v", c.pat, c.s, ok, c.want)
		}
	}
}

// An exact prefix must outrank a scattered mid-string match for the same pattern.
func TestFuzzyMatchRanksPrefixHighest(t *testing.T) {
	prefix, _, ok1 := fuzzyMatch("/s", "/status")
	mid, _, ok2 := fuzzyMatch("/s", "/tasks") // 's' is mid-word in "tasks"
	if !ok1 || !ok2 {
		t.Fatal("both should match as subsequences")
	}
	if prefix <= mid {
		t.Fatalf("prefix /status (%d) should outrank mid-string /tasks (%d)", prefix, mid)
	}
}

// The menu filters fuzzily and ranks best matches first.
func TestSlashMenuFuzzyRanks(t *testing.T) {
	m := newSlashMenu([]Command{
		{Name: "/status"}, {Name: "/sessions"}, {Name: "/tasks"}, {Name: "/models"},
	})
	m.update("/st")
	if !m.open || len(m.filtered) == 0 {
		t.Fatal("menu should be open with matches for /st")
	}
	if m.filtered[0].Name != "/status" {
		t.Fatalf("/status should rank first for /st, got %q", m.filtered[0].Name)
	}
	// A bare slash shows everything.
	m.update("/")
	if len(m.filtered) != 4 {
		t.Fatalf("/ should show all 4 commands, got %d", len(m.filtered))
	}
}
