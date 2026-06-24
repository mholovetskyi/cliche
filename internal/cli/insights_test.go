package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/ledger"
)

func TestGatherInsights(t *testing.T) {
	dir := t.TempDir()
	led, err := ledger.Open(config.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	led.Append(ledger.Entry{Event: ledger.EventTurn, InputTokens: 100, OutputTokens: 50, USD: 0.01})
	led.Append(ledger.Entry{Event: ledger.EventTool, Detail: "read_file success=true main.go"})
	led.Append(ledger.Entry{Event: ledger.EventTool, Detail: "read_file success=true x.go"})
	led.Append(ledger.Entry{Event: ledger.EventTool, Detail: "run_command success=true go test"})
	led.Append(ledger.Entry{Event: ledger.EventVerdict, Verdict: "verified"})
	led.Append(ledger.Entry{Event: ledger.EventHalt, Detail: "max_turns"})

	d := gatherInsights(dir)
	if d.turns != 1 || d.inTok != 100 || d.outTok != 50 || d.halts != 1 {
		t.Fatalf("insights = %+v", d)
	}
	if d.tools["read_file"] != 2 || d.tools["run_command"] != 1 {
		t.Fatalf("tool counts = %v", d.tools)
	}
	if d.verdicts["verified"] != 1 {
		t.Fatalf("verdicts = %v", d.verdicts)
	}

	var out bytes.Buffer
	renderInsights(&out, dir)
	for _, want := range []string{"insights", "turns", "read_file", "verified"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("rendered insights missing %q:\n%s", want, out.String())
		}
	}
}
