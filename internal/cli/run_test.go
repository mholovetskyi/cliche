package cli

import (
	"testing"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/verifier"
)

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		name string
		o    agent.Outcome
		want int
	}{
		{"completed", agent.Outcome{Stop: agent.StopCompleted}, 0},
		{"budget", agent.Outcome{Stop: agent.StopBudget}, 3},
		{"governor", agent.Outcome{Stop: "repetition"}, 4},
		{"completed+flagged", agent.Outcome{Stop: agent.StopCompleted, Verdict: verifier.StatusFlagged}, 5},
		// A budget/governor stop must win over a verdict (precedence regression).
		{"budget+flagged", agent.Outcome{Stop: agent.StopBudget, Verdict: verifier.StatusFlagged}, 3},
		{"completed+verified", agent.Outcome{Stop: agent.StopCompleted, Verdict: verifier.StatusVerified}, 0},
	}
	for _, c := range cases {
		if got := exitCodeFor(c.o); got != c.want {
			t.Errorf("%s: exitCodeFor = %d, want %d", c.name, got, c.want)
		}
	}
}
