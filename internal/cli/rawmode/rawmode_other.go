//go:build !windows && !linux && !darwin

// On an unsupported OS, raw mode is unavailable but the package must still build
// so the chat REPL degrades to cooked input instead of failing to compile.
package rawmode

import (
	"errors"
	"os"
)

// State is an empty placeholder on unsupported platforms.
type State struct{}

// Enable always fails here, so callers fall back to cooked input.
func Enable(in, out *os.File) (*State, error) {
	return nil, errors.New("raw mode unsupported on this OS")
}

// Disable is a no-op.
func (st *State) Disable() error { return nil }

// IsTerminal reports false so the cooked path is always chosen.
func IsTerminal(f *os.File) bool { return false }

// Size returns a sane default; raw rendering isn't used on unsupported OSes.
func Size(f *os.File) (cols, rows int) { return 80, 24 }
