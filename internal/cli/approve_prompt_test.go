package cli

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestApproveBracketsPromptWithStartEnd(t *testing.T) {
	starts, ends := 0, 0
	a := &approver{
		r:             bufio.NewReader(strings.NewReader("y\n")),
		out:           &bytes.Buffer{},
		onPromptStart: func() { starts++ },
		onPromptEnd:   func() { ends++ },
	}
	if !a.Approve("write", "write_file foo.go") {
		t.Fatal("y should approve")
	}
	if starts != 1 || ends != 1 {
		t.Fatalf("onPromptStart/End should each fire once around the prompt, got start=%d end=%d", starts, ends)
	}
}

func TestApproveSkipsPromptHooksWhenNoPromptShown(t *testing.T) {
	calls := 0
	hook := func() { calls++ }
	// full mode auto-approves; always-allow short-circuits — neither shows a
	// prompt, so the spinner-pause hooks must NOT fire (nothing to mask).
	full := &approver{r: bufio.NewReader(strings.NewReader("")), out: &bytes.Buffer{}, mode: modeFull, onPromptStart: hook, onPromptEnd: hook}
	if !full.Approve("write", "write_file foo.go") {
		t.Fatal("full mode should auto-approve")
	}
	always := &approver{r: bufio.NewReader(strings.NewReader("")), out: &bytes.Buffer{}, alwaysWrite: true, onPromptStart: hook, onPromptEnd: hook}
	if !always.Approve("write", "write_file foo.go") {
		t.Fatal("always-allow should approve")
	}
	if calls != 0 {
		t.Fatalf("prompt hooks must not fire when no prompt is shown, got %d", calls)
	}
}
