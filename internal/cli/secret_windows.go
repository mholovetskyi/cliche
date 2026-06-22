//go:build windows

package cli

import (
	"os"
	"syscall"
	"unsafe"
)

// Console echo is toggled through the Win32 console API (kernel32) directly, so
// hidden key entry needs no third-party dependency — consistent with the
// zero-dependency rule for the CLI.
var (
	modkernel32        = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = modkernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = modkernel32.NewProc("SetConsoleMode")
)

const enableEchoInput = 0x0004 // ENABLE_ECHO_INPUT

// withEchoDisabled runs fn with terminal echo turned off, restoring it after.
// It returns false (without running fn) when stdin is not a real console, so
// the caller can fall back to visible input.
func withEchoDisabled(fn func()) bool {
	h := syscall.Handle(os.Stdin.Fd())
	var mode uint32
	if r, _, _ := procGetConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return false // not a console
	}
	if r, _, _ := procSetConsoleMode.Call(uintptr(h), uintptr(mode&^enableEchoInput)); r == 0 {
		return false
	}
	defer procSetConsoleMode.Call(uintptr(h), uintptr(mode))
	fn()
	return true
}
