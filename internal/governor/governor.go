// Package governor implements the loop / runaway-autonomy circuit-breaker.
//
// This is the low-difficulty, universally-absent feature in the category, so
// it ships ON BY DEFAULT and strict. Every halt is structured (a HaltReason)
// so the caller can report exactly why the agent was stopped.
package governor

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Limits configure the breakers. A zero value disables that dimension.
type Limits struct {
	MaxTurns                  int
	MaxWallClock              time.Duration
	MaxConsecutiveFailedEdits int
	RepetitionWindow          int // how many recent tool-call signatures to remember
	RepetitionThreshold       int // identical signatures within the window that trip the breaker
	NoProgressTurns           int // turns with no successful edit before halting
}

// DefaultLimits are deliberately conservative-but-aggressive defaults.
func DefaultLimits() Limits {
	return Limits{
		MaxTurns:                  50,
		MaxWallClock:              30 * time.Minute,
		MaxConsecutiveFailedEdits: 5,
		RepetitionWindow:          8,
		RepetitionThreshold:       3,
		NoProgressTurns:           12,
	}
}

// ErrHalt wraps every governor stop.
var ErrHalt = errors.New("governor halt")

// HaltReason explains, in structured form, why the loop was stopped.
type HaltReason struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
	Turn   int    `json:"turn"`
}

func (h HaltReason) Error() string {
	return fmt.Sprintf("%v: %s [%s @ turn %d]", ErrHalt, h.Detail, h.Code, h.Turn)
}

// Governor tracks loop state and trips a breaker when a limit is crossed.
type Governor struct {
	limits             Limits
	start              time.Time
	turn               int
	failedEdits        int
	turnsSinceProgress int
	recent             []string
	now                func() time.Time
}

// New returns a Governor with the given limits, using the real clock.
func New(l Limits) *Governor {
	return &Governor{limits: l, start: time.Now(), now: time.Now}
}

// WithClock injects a clock (for tests). It resets the start time.
func (g *Governor) WithClock(now func() time.Time) *Governor {
	g.now = now
	g.start = now()
	return g
}

// BeginTurn increments the turn counter and checks the per-run breakers
// (max turns, wall clock). Returns the new turn number and a halt reason if
// the run must stop.
func (g *Governor) BeginTurn() (int, *HaltReason) {
	g.turn++
	if g.limits.MaxTurns > 0 && g.turn > g.limits.MaxTurns {
		return g.turn, &HaltReason{Code: "max_turns", Detail: fmt.Sprintf("reached the %d-turn limit", g.limits.MaxTurns), Turn: g.turn}
	}
	if g.limits.MaxWallClock > 0 && g.now().Sub(g.start) > g.limits.MaxWallClock {
		return g.turn, &HaltReason{Code: "max_wallclock", Detail: fmt.Sprintf("exceeded the %s wall-clock limit", g.limits.MaxWallClock), Turn: g.turn}
	}
	return g.turn, nil
}

// RecordToolCall records a tool-call signature and trips the repetition
// breaker if the same signature recurs too often in the recent window. The
// signature should be a stable description of the call (e.g. name + key args)
// so that an agent stuck re-issuing the same failing edit is detected.
func (g *Governor) RecordToolCall(signature string) *HaltReason {
	h := shortHash(signature)
	g.recent = append(g.recent, h)
	if g.limits.RepetitionWindow > 0 && len(g.recent) > g.limits.RepetitionWindow {
		g.recent = g.recent[len(g.recent)-g.limits.RepetitionWindow:]
	}
	if g.limits.RepetitionThreshold > 0 {
		count := 0
		for _, x := range g.recent {
			if x == h {
				count++
			}
		}
		if count >= g.limits.RepetitionThreshold {
			return &HaltReason{
				Code:   "repetition",
				Detail: fmt.Sprintf("identical tool call repeated %d× within the last %d calls", count, len(g.recent)),
				Turn:   g.turn,
			}
		}
	}
	return nil
}

// RecordEdit reports the outcome of an edit and trips the failed-edit breaker
// after too many consecutive failures (the loop that burns tokens).
func (g *Governor) RecordEdit(success bool) *HaltReason {
	if success {
		g.failedEdits = 0
		return nil
	}
	g.failedEdits++
	if g.limits.MaxConsecutiveFailedEdits > 0 && g.failedEdits >= g.limits.MaxConsecutiveFailedEdits {
		return &HaltReason{
			Code:   "failed_edits",
			Detail: fmt.Sprintf("%d consecutive failed edits", g.failedEdits),
			Turn:   g.turn,
		}
	}
	return nil
}

// RecordTurnProgress reports whether a turn made real progress (a successful
// edit). It trips the no-progress breaker after too many idle turns.
func (g *Governor) RecordTurnProgress(madeProgress bool) *HaltReason {
	if madeProgress {
		g.turnsSinceProgress = 0
		return nil
	}
	g.turnsSinceProgress++
	if g.limits.NoProgressTurns > 0 && g.turnsSinceProgress >= g.limits.NoProgressTurns {
		return &HaltReason{
			Code:   "no_progress",
			Detail: fmt.Sprintf("%d turns without progress", g.turnsSinceProgress),
			Turn:   g.turn,
		}
	}
	return nil
}

// Turn returns the current turn number.
func (g *Governor) Turn() int { return g.turn }

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}
