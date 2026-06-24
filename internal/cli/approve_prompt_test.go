package cli

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestApproveFiresOnPromptBeforeBlocking(t *testing.T) {
	called := 0
	a := &approver{
		r:        bufio.NewReader(strings.NewReader("y\n")),
		out:      &bytes.Buffer{},
		onPrompt: func() { called++ },
	}
	if !a.Approve("write", "write_file foo.go") {
		t.Fatal("y should approve")
	}
	if called != 1 {
		t.Fatalf("onPrompt should fire exactly once before the prompt, got %d", called)
	}
}

func TestApproveSkipsOnPromptWhenNoPromptShown(t *testing.T) {
	called := 0
	// full mode auto-approves; always-allow short-circuits — neither shows a
	// prompt, so the spinner-pause hook must NOT fire (nothing to mask).
	full := &approver{r: bufio.NewReader(strings.NewReader("")), out: &bytes.Buffer{}, mode: modeFull, onPrompt: func() { called++ }}
	if !full.Approve("write", "write_file foo.go") {
		t.Fatal("full mode should auto-approve")
	}
	always := &approver{r: bufio.NewReader(strings.NewReader("")), out: &bytes.Buffer{}, alwaysWrite: true, onPrompt: func() { called++ }}
	if !always.Approve("write", "write_file foo.go") {
		t.Fatal("always-allow should approve")
	}
	if called != 0 {
		t.Fatalf("onPrompt must not fire when no prompt is shown, got %d", called)
	}
}
