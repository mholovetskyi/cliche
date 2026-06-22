package cli

import (
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/ledger"
)

func TestRenderReportContents(t *testing.T) {
	d := reportData{
		Title:    "add a parser",
		Provider: "openrouter",
		Model:    "openai/gpt-4o-mini",
		Summary: ledger.Summary{
			Turns: 7, InputTokens: 1200, OutputTokens: 340, USD: 0.0123,
			Verdicts: map[string]int{"verified": 1},
		},
		Stat:  "3 files changed, 40 insertions(+), 2 deletions(-)",
		Files: []string{"parser.go", "parser_test.go"},
	}
	md := renderReport(d)

	for _, want := range []string{
		"## 🤖 Cliche run report",
		"**Task:** add a parser",
		"openrouter · openai/gpt-4o-mini",
		"✅ verified (1)",
		"| Turns | 7 |",
		"1200 in / 340 out (1540 total)",
		"~$0.0123",
		"3 files changed",
		"`parser.go`",
		"Trust Kernel",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report missing %q\n---\n%s", want, md)
		}
	}
}

func TestVerdictLine(t *testing.T) {
	if got := verdictLine(nil); !strings.Contains(got, "not verified") {
		t.Errorf("empty verdicts should read as not verified, got %q", got)
	}
	// flagged dominates verified.
	if got := verdictLine(map[string]int{"verified": 2, "flagged": 1}); !strings.Contains(got, "flagged") {
		t.Errorf("flagged should dominate, got %q", got)
	}
}
