package provider

import "context"

// Mock is a deterministic, offline provider for tests and the demo. It
// replays a fixed script; if loop is true it repeats the last entry forever
// (used to simulate a runaway agent).
type Mock struct {
	model  string
	script []Response
	i      int
	loop   bool
}

// NewMock returns a Mock that replays script. If loop is true, the final
// scripted response repeats indefinitely once the script is exhausted.
func NewMock(model string, script []Response, loop bool) *Mock {
	return &Mock{model: model, script: script, loop: loop}
}

func (m *Mock) Name() string  { return "mock" }
func (m *Mock) Model() string { return m.model }

// Complete returns the next scripted response.
func (m *Mock) Complete(_ context.Context, _ Request) (Response, error) {
	if m.i < len(m.script) {
		r := m.script[m.i]
		m.i++
		return r, nil
	}
	if m.loop && len(m.script) > 0 {
		return m.script[len(m.script)-1], nil
	}
	return Response{Text: "done", Done: true, Usage: Usage{InputTokens: 10, OutputTokens: 5}}, nil
}

// RunawayScript simulates the documented failure mode: an agent stuck
// re-issuing the SAME failing edit, burning tokens with no progress. With a
// Governor this trips the repetition breaker; without one it never stops.
func RunawayScript() []Response {
	return []Response{{
		Text: "Hmm, that didn't apply. Let me try the same edit again...",
		ToolCalls: []ToolCall{{
			Name:      "apply_diff",
			Args:      map[string]string{"file": "main.go"},
			Signature: "apply_diff:main.go:same-hunk",
		}},
		Usage: Usage{InputTokens: 4200, OutputTokens: 800},
	}}
}

// HeavyScript simulates a task whose turns each burn a lot of tokens with
// VARIED signatures (so repetition does not fire). Used to demonstrate the
// budget breaker, including a mid-stream catch where preflight passes but the
// actual usage blows the cap.
func HeavyScript() []Response {
	mk := func(sig string) Response {
		return Response{
			Text:      "Working on a large refactor across many files...",
			ToolCalls: []ToolCall{{Name: "read_file", Args: map[string]string{"file": sig}, Signature: "read_file:" + sig}},
			Usage:     Usage{InputTokens: 50000, OutputTokens: 10000},
		}
	}
	return []Response{mk("a.go"), mk("b.go"), mk("c.go"), mk("d.go"), mk("e.go")}
}

// NormalScript simulates a healthy short task that completes cleanly.
func NormalScript() []Response {
	return []Response{
		{
			Text:      "Reading the target file.",
			ToolCalls: []ToolCall{{Name: "read_file", Args: map[string]string{"file": "main.go"}, Signature: "read_file:main.go"}},
			Usage:     Usage{InputTokens: 1200, OutputTokens: 150},
		},
		{
			Text:      "Applying the fix.",
			ToolCalls: []ToolCall{{Name: "write_file", Args: map[string]string{"file": "main.go"}, Signature: "write_file:main.go"}},
			Usage:     Usage{InputTokens: 1500, OutputTokens: 400},
		},
		{
			Text:  "Done. The fix is applied and the tests pass.",
			Done:  true,
			Usage: Usage{InputTokens: 900, OutputTokens: 120},
		},
	}
}
