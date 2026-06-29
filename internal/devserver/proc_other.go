//go:build !windows

package devserver

import (
	"os/exec"
	"syscall"
)

// setProcGroup makes the dev server a process-group leader so the whole tree
// (npm → node → esbuild) can be signalled at once.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killTree signals the entire process group (negative pid) so children die with
// the parent instead of lingering as orphans.
func killTree(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
