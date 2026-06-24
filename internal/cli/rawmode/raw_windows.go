//go:build windows

package rawmode

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// Raw mode on Windows goes through the Win32 console API (kernel32) directly —
// the same zero-dependency idiom the repo already uses for hidden key entry
// (see internal/cli/secret_windows.go). Enabling ENABLE_VIRTUAL_TERMINAL_INPUT
// makes the console deliver arrow/edit keys as the same ESC-[ sequences as Unix,
// so the shared keydec decoder needs no OS-specific path.
var (
	modkernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode             = modkernel32.NewProc("GetConsoleMode")
	procSetConsoleMode             = modkernel32.NewProc("SetConsoleMode")
	procGetConsoleScreenBufferInfo = modkernel32.NewProc("GetConsoleScreenBufferInfo")
)

type coord struct{ x, y int16 }
type smallRect struct{ left, top, right, bottom int16 }
type consoleScreenBufferInfo struct {
	size              coord
	cursorPosition    coord
	attributes        uint16
	window            smallRect
	maximumWindowSize coord
}

const (
	enableProcessedInput            = 0x0001
	enableLineInput                 = 0x0002
	enableEchoInput                 = 0x0004
	enableVirtualTerminalInput      = 0x0200
	enableProcessedOutput           = 0x0001
	enableVirtualTerminalProcessing = 0x0004
)

type termState struct {
	inHandle, outHandle syscall.Handle
	inMode, outMode     uint32
}

func getConsoleMode(h syscall.Handle) (uint32, bool) {
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode)))
	return mode, r != 0
}

func setConsoleMode(h syscall.Handle, mode uint32) bool {
	r, _, _ := procSetConsoleMode.Call(uintptr(h), uintptr(mode))
	return r != 0
}

func enableRaw(in, out *os.File) (*termState, error) {
	inH, outH := syscall.Handle(in.Fd()), syscall.Handle(out.Fd())
	inMode, ok := getConsoleMode(inH)
	if !ok {
		return nil, fmt.Errorf("stdin is not a console")
	}
	outMode, ok := getConsoleMode(outH)
	if !ok {
		return nil, fmt.Errorf("stdout is not a console")
	}
	// Input: we echo and edit ourselves, and want Ctrl-C/keys as raw bytes + VT.
	// VT input is best-effort: a legacy console (pre-Win10 1809 / some conhost)
	// rejects the ENABLE_VIRTUAL_TERMINAL_INPUT bit and fails the whole call, which
	// would silently drop the line editor. Retry without it so basic raw editing
	// still works (keydec degrades unrecognized input safely).
	raw := inMode
	raw &^= enableEchoInput | enableLineInput | enableProcessedInput
	if !setConsoleMode(inH, raw|enableVirtualTerminalInput) {
		if !setConsoleMode(inH, raw) {
			return nil, fmt.Errorf("could not set console input mode")
		}
	}
	// Output: ensure our ANSI escape sequences render.
	if !setConsoleMode(outH, outMode|enableProcessedOutput|enableVirtualTerminalProcessing) {
		setConsoleMode(inH, inMode) // restore input before bailing out
		return nil, fmt.Errorf("could not set console output mode")
	}
	return &termState{inHandle: inH, outHandle: outH, inMode: inMode, outMode: outMode}, nil
}

func disableRaw(ts *termState) error {
	setConsoleMode(ts.inHandle, ts.inMode)
	setConsoleMode(ts.outHandle, ts.outMode)
	return nil
}

func isTerminal(f *os.File) bool {
	_, ok := getConsoleMode(syscall.Handle(f.Fd()))
	return ok
}

// termSize reads the visible console window size (srWindow, not the larger
// buffer). Defaults to 80x24 on failure.
func termSize(f *os.File) (int, int) {
	var info consoleScreenBufferInfo
	r, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(syscall.Handle(f.Fd())), uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return 80, 24
	}
	cols := int(info.window.right-info.window.left) + 1
	rows := int(info.window.bottom-info.window.top) + 1
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	return cols, rows
}
