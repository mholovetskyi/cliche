package cli

import "testing"

func TestMaxOutputFor(t *testing.T) {
	big := []string{"claude-sonnet-4-6", "claude-opus-4-8", "claude-fable-5", "gpt-5", "gemini-2.5-pro", "deepseek-chat"}
	for _, m := range big {
		if got := maxOutputFor(m); got < 32768 {
			t.Fatalf("%s should get a large output cap, got %d", m, got)
		}
	}
	if maxOutputFor("gpt-4o") != 16384 {
		t.Fatalf("gpt-4o want 16384, got %d", maxOutputFor("gpt-4o"))
	}
	if maxOutputFor("some-unknown-model") != 8192 {
		t.Fatalf("unknown model want 8192 floor, got %d", maxOutputFor("some-unknown-model"))
	}
}
