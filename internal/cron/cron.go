// Package cron is a tiny, zero-dependency cron parser + next-fire calculator for
// Cliche's scheduler. It supports standard 5-field specs
// (minute hour day-of-month month day-of-week), the usual @shortcuts, and
// "@every <duration>" for simple intervals. Vixie semantics: when BOTH
// day-of-month and day-of-week are restricted, a day matches if EITHER does.
package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a parsed cron spec.
type Schedule struct {
	every                    time.Duration // non-zero → interval mode ("@every 30m")
	min, hour, dom, mon, dow uint64        // bitsets of allowed values
	domStar, dowStar         bool          // whether dom / dow were "*"
	spec                     string        // original, for display
}

// String returns the original spec.
func (s Schedule) String() string { return s.spec }

var shortcuts = map[string]string{
	"@hourly":   "0 * * * *",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@weekly":   "0 0 * * 0",
	"@monthly":  "0 0 1 * *",
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
}

// Parse parses a cron spec, returning an error for anything malformed (so a bad
// schedule is rejected at `cron add`, never at fire time).
func Parse(spec string) (Schedule, error) {
	raw := strings.TrimSpace(spec)
	if raw == "" {
		return Schedule{}, fmt.Errorf("empty schedule")
	}
	if strings.HasPrefix(raw, "@every ") {
		d, err := time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(raw, "@every ")))
		if err != nil {
			return Schedule{}, fmt.Errorf("@every: %v", err)
		}
		if d < time.Minute {
			return Schedule{}, fmt.Errorf("@every interval must be at least 1m")
		}
		return Schedule{every: d, spec: raw}, nil
	}
	expr := raw
	if sc, ok := shortcuts[raw]; ok {
		expr = sc
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return Schedule{}, fmt.Errorf("expected 5 fields (min hour dom month dow) or an @shortcut, got %d", len(fields))
	}
	s := Schedule{spec: raw}
	var err error
	if s.min, _, err = parseField(fields[0], 0, 59); err != nil {
		return Schedule{}, fmt.Errorf("minute: %v", err)
	}
	if s.hour, _, err = parseField(fields[1], 0, 23); err != nil {
		return Schedule{}, fmt.Errorf("hour: %v", err)
	}
	if s.dom, s.domStar, err = parseField(fields[2], 1, 31); err != nil {
		return Schedule{}, fmt.Errorf("day-of-month: %v", err)
	}
	if s.mon, _, err = parseField(fields[3], 1, 12); err != nil {
		return Schedule{}, fmt.Errorf("month: %v", err)
	}
	if s.dow, s.dowStar, err = parseField(fields[4], 0, 7); err != nil {
		return Schedule{}, fmt.Errorf("day-of-week: %v", err)
	}
	if s.dow&(1<<7) != 0 { // Vixie: 7 is an alias for Sunday → fold onto 0
		s.dow = (s.dow &^ (1 << 7)) | 1
	}
	return s, nil
}

// parseField parses one cron field into a bitset, also reporting whether it was
// the wildcard "*" (needed for the dom/dow OR rule).
func parseField(field string, lo, hi int) (uint64, bool, error) {
	var bits uint64
	star := false
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return 0, false, fmt.Errorf("empty term")
		}
		step := 1
		rng := part
		if slash := strings.IndexByte(part, '/'); slash >= 0 {
			rng = part[:slash]
			n, err := strconv.Atoi(part[slash+1:])
			if err != nil || n <= 0 {
				return 0, false, fmt.Errorf("bad step %q", part)
			}
			step = n
		}
		start, end := lo, hi
		switch {
		case rng == "*":
			// A bare "*" is a true wildcard; "*/n" is a RESTRICTION (every nth),
			// so only the former sets the star fast-path used by the dom/dow rule.
			if step == 1 {
				star = true
			}
		case strings.IndexByte(rng, '-') > 0:
			a, b, ok := strings.Cut(rng, "-")
			ai, err1 := strconv.Atoi(strings.TrimSpace(a))
			bi, err2 := strconv.Atoi(strings.TrimSpace(b))
			if err1 != nil || err2 != nil {
				return 0, false, fmt.Errorf("bad range %q", rng)
			}
			start, end, _ = ai, bi, ok
		default:
			n, err := strconv.Atoi(rng)
			if err != nil {
				return 0, false, fmt.Errorf("bad value %q", rng)
			}
			start, end = n, n
		}
		if start < lo || end > hi || start > end {
			return 0, false, fmt.Errorf("%d-%d out of range %d-%d", start, end, lo, hi)
		}
		for v := start; v <= end; v += step {
			bits |= 1 << uint(v)
		}
	}
	return bits, star, nil
}

// Next returns the first fire time strictly after `after`, or the zero time if
// none occurs within ~5 years (an impossible spec like Feb 30).
func (s Schedule) Next(after time.Time) time.Time {
	if s.every > 0 {
		return after.Add(s.every)
	}
	// Start at the next whole minute after `after`.
	t := after.Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(5, 0, 0)
	for t.Before(limit) {
		if s.matches(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}

func (s Schedule) matches(t time.Time) bool {
	if s.min&(1<<uint(t.Minute())) == 0 {
		return false
	}
	if s.hour&(1<<uint(t.Hour())) == 0 {
		return false
	}
	if s.mon&(1<<uint(int(t.Month()))) == 0 {
		return false
	}
	domHit := s.dom&(1<<uint(t.Day())) != 0
	dowHit := s.dow&(1<<uint(int(t.Weekday()))) != 0
	switch {
	case s.domStar && s.dowStar:
		return true
	case s.domStar:
		return dowHit
	case s.dowStar:
		return domHit
	default:
		return domHit || dowHit // Vixie: both restricted → OR
	}
}
