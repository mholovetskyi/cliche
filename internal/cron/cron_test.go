package cron

import (
	"testing"
	"time"
)

func TestParseErrors(t *testing.T) {
	bad := []string{"", "* * * *", "60 * * * *", "* 24 * * *", "* * 0 * *", "* * * 13 *", "* * * * 7", "a * * * *", "*/0 * * * *", "5-2 * * * *", "@every 30s", "@every nope"}
	for _, b := range bad {
		if _, err := Parse(b); err == nil {
			t.Errorf("Parse(%q) should have errored", b)
		}
	}
	good := []string{"0 0 * * *", "*/15 * * * *", "0 9-17 * * 1-5", "0,30 * * * *", "@daily", "@hourly", "@every 1h30m", "0 0 1 1 *"}
	for _, g := range good {
		if _, err := Parse(g); err != nil {
			t.Errorf("Parse(%q) errored: %v", g, err)
		}
	}
}

func mustNext(t *testing.T, spec, from string) time.Time {
	t.Helper()
	s, err := Parse(spec)
	if err != nil {
		t.Fatalf("Parse(%q): %v", spec, err)
	}
	f, _ := time.Parse(time.RFC3339, from)
	return s.Next(f)
}

func TestNext(t *testing.T) {
	cases := []struct{ spec, from, want string }{
		{"0 0 * * *", "2026-01-01T12:00:00Z", "2026-01-02T00:00:00Z"},    // next midnight
		{"*/15 * * * *", "2026-01-01T12:07:00Z", "2026-01-01T12:15:00Z"}, // next quarter
		{"0 9 * * 1-5", "2026-01-03T12:00:00Z", "2026-01-05T09:00:00Z"},  // Sat → Mon 9am
		{"@hourly", "2026-01-01T12:30:00Z", "2026-01-01T13:00:00Z"},
		{"0 0 13 * 5", "2026-02-01T00:00:00Z", "2026-02-06T00:00:00Z"}, // dom=13 OR dow=Fri → first Friday Feb 6
	}
	for _, c := range cases {
		got := mustNext(t, c.spec, c.from).UTC().Format(time.RFC3339)
		if got != c.want {
			t.Errorf("Next(%q from %s) = %s, want %s", c.spec, c.from, got, c.want)
		}
	}
}

func TestEveryInterval(t *testing.T) {
	got := mustNext(t, "@every 30m", "2026-01-01T12:00:00Z").UTC().Format(time.RFC3339)
	if got != "2026-01-01T12:30:00Z" {
		t.Errorf("@every 30m = %s", got)
	}
}

func TestImpossibleSpecReturnsZero(t *testing.T) {
	// Feb 30 never happens.
	s, _ := Parse("0 0 30 2 *")
	f, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
	if !s.Next(f).IsZero() {
		t.Error("impossible spec should return the zero time")
	}
}
