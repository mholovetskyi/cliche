package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// approver implements interactive y/N/always permission prompts, reading from
// a shared bufio.Reader so it coexists with an interactive session's prompt
// loop (single-threaded). "always" sticks for the rest of the process.
type approver struct {
	r           *bufio.Reader
	out         io.Writer
	alwaysWrite bool
	alwaysRun   bool
}

// Approve is passed to tools.OSExecutor as its Approver.
func (a *approver) Approve(action, detail string) bool {
	switch action {
	case "write":
		if a.alwaysWrite {
			return true
		}
	case "run":
		if a.alwaysRun {
			return true
		}
	}
	fmt.Fprintf(a.out, "  %s allow %s? (%s) [y/N/a=always] ", gl("⚠", "!"), action, detail)
	line, err := a.r.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	case "a", "always":
		if action == "write" {
			a.alwaysWrite = true
		} else {
			a.alwaysRun = true
		}
		return true
	default:
		return false
	}
}
