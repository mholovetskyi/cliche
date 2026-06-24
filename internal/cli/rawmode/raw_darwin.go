//go:build darwin

package rawmode

import "syscall"

// macOS/BSD ioctl requests for reading/writing termios (TIOCGETA/TIOCSETA).
const (
	ioctlReadTermios  = syscall.TIOCGETA
	ioctlWriteTermios = syscall.TIOCSETA
)
