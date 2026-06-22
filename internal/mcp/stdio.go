package mcp

import (
	"context"
	"io"
	"os"
	"os/exec"
)

type stdioCloser struct {
	cmd   *exec.Cmd
	stdin io.Closer
}

func (s stdioCloser) Close() error {
	_ = s.stdin.Close() // signal EOF so a well-behaved server exits
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_ = s.cmd.Wait()
	return nil
}

// StartStdio launches an MCP server subprocess and returns a Client speaking to
// it over stdio. The server's stderr is forwarded for visibility. The provided
// context bounds the process lifetime.
func StartStdio(ctx context.Context, name, command string, args, env []string) (*Client, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return NewClient(name, stdin, stdout, stdioCloser{cmd: cmd, stdin: stdin}), nil
}
