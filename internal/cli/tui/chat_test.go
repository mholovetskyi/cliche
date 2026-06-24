package tui

import (
	"strings"
	"testing"
)

func TestChatViewWriteCommitsLines(t *testing.T) {
	v := &ChatView{}
	v.Write([]byte("line one\nline two\npart"))
	if len(v.Transcript) != 2 {
		t.Fatalf("two complete lines should commit, got %d: %v", len(v.Transcript), v.Transcript)
	}
	// The in-progress "part" shows via lines() before its newline.
	if got := v.lines(); got[len(got)-1] != "part" {
		t.Fatalf("partial line should appear live, got %q", got[len(got)-1])
	}
	v.FlushPartial()
	if len(v.Transcript) != 3 {
		t.Fatalf("FlushPartial should commit the partial, got %d", len(v.Transcript))
	}
}

func TestChatViewRenderComposesFrame(t *testing.T) {
	v := &ChatView{Sidebar: []string{"mode suggest", "spend $0.01"}}
	v.Write([]byte("agent says hi\nstreaming now"))
	v.Input = "my next prompt"

	frame := v.Render(70, 14)
	if len(frame) != 14 {
		t.Fatalf("frame should be exactly height (14), got %d", len(frame))
	}
	joined := strings.Join(frame, "\n")
	for _, want := range []string{"cliché", "chat", "session", "agent says hi", "streaming now", "mode suggest", "my next prompt"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("frame missing %q:\n%s", want, joined)
		}
	}
}

func TestChatViewTranscriptTails(t *testing.T) {
	v := &ChatView{}
	for i := 0; i < 100; i++ {
		v.Write([]byte("L\n"))
	}
	// A small frame shows only the tail, never more lines than the pane holds.
	frame := v.Render(40, 8)
	if len(frame) != 8 {
		t.Fatalf("height %d want 8", len(frame))
	}
}
