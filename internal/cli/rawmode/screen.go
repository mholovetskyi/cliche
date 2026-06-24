package rawmode

import "io"

// These are pure VT escape sequences (no syscalls), so they live without a build
// constraint and work anywhere raw mode + VT output are active (every platform
// the editor runs on). They drive the full-screen TUI: the alternate screen
// buffer for a clean canvas, and SGR mouse reporting for clicks/scroll.

// EnterAlt switches to the alternate screen buffer (cleared, cursor home) and
// hides the cursor; LeaveAlt restores the primary buffer, its prior contents,
// and the cursor. Always pair them (defer LeaveAlt).
func EnterAlt(w io.Writer) { io.WriteString(w, "\x1b[?1049h\x1b[2J\x1b[H\x1b[?25l") }

// LeaveAlt restores the primary screen and cursor.
func LeaveAlt(w io.Writer) { io.WriteString(w, "\x1b[?25h\x1b[?1049l") }

// EnableMouse turns on mouse reporting: 1000 (button press/release) + 1006 (SGR
// extended coordinates, so columns/rows past 223 are reported correctly).
func EnableMouse(w io.Writer) { io.WriteString(w, "\x1b[?1000h\x1b[?1006h") }

// DisableMouse reverses EnableMouse.
func DisableMouse(w io.Writer) { io.WriteString(w, "\x1b[?1006l\x1b[?1000l") }
