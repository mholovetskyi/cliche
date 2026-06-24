//go:build linux

package rawmode

import "syscall"

// Linux ioctl requests for reading/writing termios (glibc TCGETS/TCSETS).
const (
	ioctlReadTermios  = syscall.TCGETS
	ioctlWriteTermios = syscall.TCSETS
)
