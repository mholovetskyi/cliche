package tools

import (
	"context"
	"strings"
	"testing"
)

func TestPreToolHookBlocks(t *testing.T) {
	root := t.TempDir()
	var saw string
	e := OSExecutor{
		Root:   root,
		Policy: Policy{Yolo: true},
		PreToolHook: func(name string, args map[string]string) (bool, string) {
			saw = name
			if name == "run_command" {
				return false, "commands are not allowed here"
			}
			return true, ""
		},
	}
	// A blocked tool returns the hook's reason and never runs.
	r := e.Execute(context.Background(), "run_command", map[string]string{"command": "echo hi"})
	if r.Success {
		t.Fatal("pre-tool-use hook should have blocked run_command")
	}
	if saw != "run_command" {
		t.Fatalf("hook should have seen the tool name, got %q", saw)
	}
	if !strings.Contains(r.Output, "commands are not allowed here") {
		t.Fatalf("blocked output should carry the hook reason, got %q", r.Output)
	}
	// A write the hook allows still proceeds.
	if r := e.Execute(context.Background(), "write_file", map[string]string{"file": "a.txt", "content": "hi"}); !r.Success {
		t.Fatalf("hook-allowed write should succeed: %s", r.Output)
	}
}
