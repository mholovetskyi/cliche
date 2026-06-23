package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/style"
	"github.com/mholovetskyi/cliche/internal/verifier"
)

func TestHumanHelpers(t *testing.T) {
	if got := humanDuration(450 * time.Millisecond); got != "<1s" {
		t.Errorf("humanDuration sub-second = %q", got)
	}
	if got := humanDuration(14 * time.Second); got != "14s" {
		t.Errorf("humanDuration = %q", got)
	}
	if got := humanDuration(95 * time.Second); got != "1m35s" {
		t.Errorf("humanDuration minutes = %q", got)
	}
	if got := humanTokens(842); got != "842" {
		t.Errorf("humanTokens small = %q", got)
	}
	if got := humanTokens(8421); got != "8.4k" {
		t.Errorf("humanTokens k = %q", got)
	}
	if pluralTurns(1) != "1 turn" || pluralTurns(3) != "3 turns" {
		t.Error("pluralTurns wrong")
	}
}

func TestHumanStopHumanizesGovernorCodes(t *testing.T) {
	for _, code := range []string{"max_turns", "max_wallclock", "repetition", "failed_edits", "no_progress", agent.StopBudget} {
		if h := humanStop(agent.Outcome{Stop: code}); h == "" || h == code {
			t.Errorf("humanStop(%q) should produce a plain-English remedy, got %q", code, h)
		}
	}
	if humanStop(agent.Outcome{Stop: agent.StopCancelled}) != "" {
		t.Error("an interrupt is self-evident; no remedy line")
	}
}

func TestVerdictStyledDistinctUnderNoColor(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = false, true
	defer func() { style.Enabled, noColor = oldE, oldNC }()
	if got := verdictStyled(verifier.StatusFlagged); !strings.Contains(got, "FLAGGED") {
		t.Fatalf("flagged must be distinct (uppercase) under NO_COLOR, got %q", got)
	}
	if got := verdictStyled(verifier.StatusVerified); !strings.Contains(got, "verified") {
		t.Fatalf("verified verdict text missing: %q", got)
	}
}

func TestRenderOutcomeContent(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = false, true
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	var done bytes.Buffer
	renderOutcome(&done, agent.Outcome{Stop: agent.StopCompleted, Turns: 3}, outcomeMetrics{elapsed: 14 * time.Second, tokens: 8421, taskUSD: 0.0042, sessionUSD: 0.0312})
	s := done.String()
	for _, want := range []string{"done", "3 turns", "14s", "8.4k tokens", "$0.0042 this task", "$0.0312 session"} {
		if !strings.Contains(s, want) {
			t.Errorf("completed outcome missing %q:\n%s", want, s)
		}
	}

	var budget bytes.Buffer
	renderOutcome(&budget, agent.Outcome{Stop: agent.StopBudget, Turns: 5}, outcomeMetrics{tokens: 100, taskUSD: 0.5, sessionUSD: -1})
	if !strings.Contains(budget.String(), "spend/token cap") {
		t.Errorf("budget halt should carry a humanized remedy:\n%s", budget.String())
	}
}

func TestRenderOutcomeLeadsWithVerdictAndFindings(t *testing.T) {
	oldE, oldNC := style.Enabled, noColor
	style.Enabled, noColor = false, true
	defer func() { style.Enabled, noColor = oldE, oldNC }()

	var buf bytes.Buffer
	renderOutcome(&buf, agent.Outcome{Stop: agent.StopCompleted, Turns: 2, Verdict: verifier.StatusFlagged},
		outcomeMetrics{tokens: 100, taskUSD: 0.01, sessionUSD: -1,
			findings: []verifier.Finding{{Rule: "tests_failed", Detail: "independent re-run failed"}}})
	s := buf.String()
	if !strings.Contains(s, "FLAGGED") {
		t.Fatalf("outcome should show the verdict:\n%s", s)
	}
	if !strings.Contains(s, "tests_failed") || !strings.Contains(s, "independent re-run failed") {
		t.Fatalf("flagged outcome should surface the finding rule + detail:\n%s", s)
	}
	// The verdict must lead — appear before the status badge ("done").
	if vi, di := strings.Index(s, "FLAGGED"), strings.Index(s, "done"); vi < 0 || di < 0 || vi > di {
		t.Fatalf("verdict should lead the outcome, before the status line:\n%s", s)
	}
}
