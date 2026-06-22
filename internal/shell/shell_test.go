package shell

import (
	"context"
	"testing"
)

func TestCommandExitCodePropagates(t *testing.T) {
	if err := Command(context.Background(), "", "exit 0").Run(); err != nil {
		t.Fatalf("exit 0 should succeed, got %v", err)
	}
	if err := Command(context.Background(), "", "exit 3").Run(); err == nil {
		t.Fatal("exit 3 should report a failure")
	}
}

func TestCommandSetsDir(t *testing.T) {
	dir := t.TempDir()
	if c := Command(context.Background(), dir, "exit 0"); c.Dir != dir {
		t.Fatalf("Dir not set: %q", c.Dir)
	}
}

func TestDescribeNonEmpty(t *testing.T) {
	if Describe() == "" {
		t.Fatal("Describe() must report the active shell")
	}
}
