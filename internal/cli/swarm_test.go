package cli

import (
	"bytes"
	"context"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/mholovetskyi/cliche/internal/agent"
)

func TestParsePlan(t *testing.T) {
	cases := []struct {
		name, in string
		want     []string
	}{
		{"json array", `["do A", "do B"]`, []string{"do A", "do B"}},
		{"json object", `{"subtasks": ["x", "y", "z"]}`, []string{"x", "y", "z"}},
		{"json amid prose", "Here is the plan:\n[\"a\", \"b\"]\nthanks", []string{"a", "b"}},
		{"numbered", "1. first\n2) second\n3. third", []string{"first", "second", "third"}},
		{"bulleted", "- alpha\n* beta", []string{"alpha", "beta"}},
		{"degenerate", "just do the whole thing", []string{"just do the whole thing"}},
		{"empty", "   ", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parsePlan(c.in); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("parsePlan(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestSwarmPipeline(t *testing.T) {
	var mu sync.Mutex
	roles := map[string]int{}
	var execPrompts []string
	run := func(_ context.Context, system, prompt string) (agent.Outcome, error) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.Contains(system, "PLANNER"):
			roles["planner"]++
			return agent.Outcome{Stop: agent.StopCompleted, Reason: `["task A", "task B"]`, Turns: 2}, nil
		case strings.Contains(system, "EXECUTOR"):
			roles["executor"]++
			execPrompts = append(execPrompts, prompt)
			return agent.Outcome{Stop: agent.StopCompleted, Reason: "did " + prompt, Turns: 3}, nil
		default: // SYNTHESIZER
			roles["synth"]++
			return agent.Outcome{Stop: agent.StopCompleted, Reason: "FINAL: " + prompt, Turns: 1}, nil
		}
	}

	sw := &swarm{run: run, out: &bytes.Buffer{}, maxSub: 5}
	final, outcome, err := sw.execute(context.Background(), "the big task")
	if err != nil {
		t.Fatal(err)
	}
	if roles["planner"] != 1 || roles["executor"] != 2 || roles["synth"] != 1 {
		t.Fatalf("role calls = %v, want 1 planner / 2 executor / 1 synth", roles)
	}
	if !strings.HasPrefix(final, "FINAL:") {
		t.Fatalf("final = %q, want a synthesized answer", final)
	}
	if !strings.Contains(final, "did task A") || !strings.Contains(final, "did task B") {
		t.Fatalf("synthesis should carry both executor results:\n%s", final)
	}
	if outcome.Stop != agent.StopCompleted {
		t.Fatalf("outcome.Stop = %q", outcome.Stop)
	}
	// The summary must reflect the whole swarm: planner(2) + 2×executor(3) + synth(1).
	if outcome.Turns != 2+3+3+1 {
		t.Fatalf("outcome.Turns = %d, want 9 (aggregated across the swarm)", outcome.Turns)
	}

	mu.Lock()
	got := append([]string(nil), execPrompts...)
	mu.Unlock()
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"task A", "task B"}) {
		t.Fatalf("executors received %v, want the two subtasks", got)
	}
}

func TestSwarmCapsSubtasks(t *testing.T) {
	var mu sync.Mutex
	execs := 0
	run := func(_ context.Context, system, _ string) (agent.Outcome, error) {
		switch {
		case strings.Contains(system, "PLANNER"):
			return agent.Outcome{Stop: agent.StopCompleted, Reason: `["a","b","c","d","e","f","g"]`}, nil
		case strings.Contains(system, "EXECUTOR"):
			mu.Lock()
			execs++
			mu.Unlock()
			return agent.Outcome{Stop: agent.StopCompleted, Reason: "ok"}, nil
		default:
			return agent.Outcome{Stop: agent.StopCompleted, Reason: "final"}, nil
		}
	}
	sw := &swarm{run: run, out: &bytes.Buffer{}, maxSub: 5}
	if _, _, err := sw.execute(context.Background(), "t"); err != nil {
		t.Fatal(err)
	}
	if execs != 5 {
		t.Fatalf("executors ran %d times, want 5 (capped from 7)", execs)
	}
}
