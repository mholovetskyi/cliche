//go:build linux || darwin

package rawmode

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// Raw mode on Unix is termios via ioctl, stdlib-only. The body is shared across
// Linux and macOS; the only difference is the two ioctl request constants, which
// live in raw_linux.go / raw_darwin.go (they are OS-specific symbols in the
// stdlib syscall package, so they cannot be named in one file). All flag/index
// values use untyped syscall.* symbols, so the Linux (uint32) vs Darwin (uint64)
// Termios field-width difference is transparent.

type termState struct {
	fd  uintptr
	old syscall.Termios
}

func ioctl(fd, req uintptr, t *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(unsafe.Pointer(t)))
	if errno != 0 {
		return errno
	}
	return nil
}

func enableRaw(in, out *os.File) (*termState, error) {
	fd := in.Fd()
	var old syscall.Termios
	if err := ioctl(fd, ioctlReadTermios, &old); err != nil {
		return nil, fmt.Errorf("stdin is not a terminal: %w", err)
	}
	raw := old // copy
	raw.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := ioctl(fd, ioctlWriteTermios, &raw); err != nil {
		return nil, err
	}
	return &termState{fd: fd, old: old}, nil
}

func disableRaw(ts *termState) error {
	old := ts.old // copy before writing back (no aliasing)
	return ioctl(ts.fd, ioctlWriteTermios, &old)
}

func isTerminal(f *os.File) bool {
	var t syscall.Termios
	return ioctl(f.Fd(), ioctlReadTermios, &t) == nil
}
