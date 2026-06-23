package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/style"
)

func TestTaskSurface(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = false, true
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	var out bytes.Buffer
	s := &session{out: &out} // no id → persist is a no-op

	s.addTask("/plan write the parser")
	s.addTask("/plan add tests")
	if len(s.tasks) != 2 || s.tasks[0].ID != 1 || s.tasks[1].ID != 2 {
		t.Fatalf("two tasks with ids 1,2 expected, got %+v", s.tasks)
	}

	s.markTaskDone("/done 1")
	if !s.tasks[0].Done {
		t.Fatal("/done 1 should complete task #1")
	}

	out.Reset()
	s.showTasks()
	got := out.String()
	for _, want := range []string{"plan", "1/2 done", "write the parser", "add tests"} {
		if !strings.Contains(got, want) {
			t.Errorf("/tasks missing %q:\n%s", want, got)
		}
	}

	out.Reset()
	s.markTaskDone("/done 99")
	if !strings.Contains(out.String(), "no task #99") {
		t.Fatalf("/done with an unknown id should report it:\n%s", out.String())
	}

	// Empty plan and a missing argument both guide rather than crash.
	out.Reset()
	(&session{out: &out}).showTasks()
	if !strings.Contains(out.String(), "no tasks yet") {
		t.Fatalf("empty /tasks should prompt to add one:\n%s", out.String())
	}
	out.Reset()
	s.addTask("/plan")
	if !strings.Contains(out.String(), "usage:") {
		t.Fatalf("/plan with no description should show usage:\n%s", out.String())
	}
}
