//go:build windows

package devserver

import (
	"os/exec"
	"strconv"
	"syscall"
)

// setProcGroup puts the dev server in its own process group so we can later kill
// the whole tree (npm → node → esbuild/vite) rather than orphaning children.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200} // CREATE_NEW_PROCESS_GROUP
}

// killTree terminates the process and every descendant. taskkill /T walks the
// tree; /F forces it. (Plain Process.Kill leaves node/vite children running.)
func killTree(pid int) {
	_ = exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
}
