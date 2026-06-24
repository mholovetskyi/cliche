//go:build windows || linux || darwin

// Package rawmode toggles a terminal between cooked and raw mode using ONLY the
// Go standard library, so the line editor receives keystrokes byte-by-byte (and
// arrow/edit keys as VT escape sequences). The per-OS files hold the syscall
// specifics; this file is the single public facade. On any unsupported OS,
// rawmode_other.go provides stubs so the build still succeeds and callers fall
// back to cooked input.
package rawmode

import "os"

// State holds the saved terminal modes needed to restore cooked mode.
type State struct{ s *termState }

// Enable puts the in/out terminals into raw mode and returns a State that
// restores them. A non-nil error means raw mode is unavailable — the caller
// should fall back to cooked input.
func Enable(in, out *os.File) (*State, error) {
	ts, err := enableRaw(in, out)
	if err != nil {
		return nil, err
	}
	return &State{s: ts}, nil
}

// Disable restores the terminal modes saved at Enable. Safe on a nil State.
func (st *State) Disable() error {
	if st == nil || st.s == nil {
		return nil
	}
	return disableRaw(st.s)
}

// IsTerminal reports whether f is a real interactive terminal.
func IsTerminal(f *os.File) bool { return isTerminal(f) }
