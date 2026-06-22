//go:build !windows

package cli

import (
	"os"
	"os/exec"
)

// withEchoDisabled turns terminal echo off via stty on the controlling terminal
// (zero-dependency), restoring it after fn. It returns false (without running
// fn) when echo can't be toggled — e.g. stdin isn't a tty — so the caller can
// fall back to visible input.
func withEchoDisabled(fn func()) bool {
	off := exec.Command("stty", "-echo")
	off.Stdin = os.Stdin
	if err := off.Run(); err != nil {
		return false
	}
	defer func() {
		on := exec.Command("stty", "echo")
		on.Stdin = os.Stdin
		_ = on.Run()
	}()
	fn()
	return true
}
