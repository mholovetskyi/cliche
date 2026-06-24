package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/ledger"
	sess "github.com/mholovetskyi/cliche/internal/session"
	"github.com/mholovetskyi/cliche/internal/style"
)

// insightsData aggregates a project's audit ledger + saved sessions for the
// usage report. All metadata — no secrets or file contents (the ledger never
// captures those).
type insightsData struct {
	sessions int
	turns    int
	inTok    int
	outTok   int
	usd      float64
	verdicts map[string]int
	tools    map[string]int
	halts    int
}

func gatherInsights(root string) insightsData {
	d := insightsData{verdicts: map[string]int{}, tools: map[string]int{}}
	metas, _ := sess.List(root)
	d.sessions = len(metas)
	f, err := os.Open(filepath.Join(config.Dir(root), "ledger.jsonl"))
	if err != nil {
		return d
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for dec.More() {
		var e ledger.Entry
		if dec.Decode(&e) != nil {
			break // a truncated tail entry shouldn't sink the whole report
		}
		d.inTok += e.InputTokens
		d.outTok += e.OutputTokens
		d.usd += e.USD
		switch e.Event {
		case ledger.EventTurn:
			d.turns++
		case ledger.EventTool:
			if fields := strings.Fields(e.Detail); len(fields) > 0 {
				d.tools[fields[0]]++ // the tool name is the first token of Detail
			}
		case ledger.EventHalt:
			d.halts++
		}
		if e.Verdict != "" {
			d.verdicts[e.Verdict]++
		}
	}
	return d
}

func renderInsights(out io.Writer, root string) {
	d := gatherInsights(root)
	fmt.Fprintln(out, "\n  "+style.BoldWhite("insights")+style.Gray("  ·  this project's ledger + sessions"))
	line := func(label, value string) {
		fmt.Fprintf(out, "  %s %s\n", style.Gray(style.Pad(label, 12)), style.White(value))
	}
	line("sessions", fmt.Sprintf("%d", d.sessions))
	line("turns", fmt.Sprintf("%d", d.turns))
	line("tokens", fmt.Sprintf("%s in · %s out", humanTokens(d.inTok), humanTokens(d.outTok)))
	line("spend", fmt.Sprintf("$%.4f", d.usd))
	if d.halts > 0 {
		line("halts", fmt.Sprintf("%d", d.halts))
	}
	if len(d.verdicts) > 0 {
		line("verdicts", joinCounts(d.verdicts))
	}
	if len(d.tools) > 0 {
		fmt.Fprintln(out, "  "+style.Gray("top tools"))
		for _, kv := range topCounts(d.tools, 8) {
			fmt.Fprintf(out, "    %s %s\n", style.White(style.Pad(kv.k, 14)), style.Gray(fmt.Sprintf("%d", kv.v)))
		}
	}
	if d.sessions == 0 && d.turns == 0 {
		fmt.Fprintln(out, "  "+style.Gray("nothing recorded yet — run a task first"))
	}
}

func (s *session) showInsights()                   { renderInsights(s.out, s.dir) }
func cmdInsights(_ []string, out, _ io.Writer) int { renderInsights(out, "."); return 0 }

type countKV struct {
	k string
	v int
}

// topCounts returns the map's entries sorted by count (desc), then name, capped.
func topCounts(m map[string]int, n int) []countKV {
	out := make([]countKV, 0, len(m))
	for k, v := range m {
		out = append(out, countKV{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].v != out[j].v {
			return out[i].v > out[j].v
		}
		return out[i].k < out[j].k
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func joinCounts(m map[string]int) string {
	parts := make([]string, 0, len(m))
	for _, kv := range topCounts(m, 99) {
		parts = append(parts, fmt.Sprintf("%s %d", kv.k, kv.v))
	}
	return strings.Join(parts, " · ")
}
