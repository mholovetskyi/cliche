package cli

import (
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/ledger"
)

// cmdCost summarizes the append-only ledger for a project.
func cmdCost(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("cost", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	led, err := ledger.Open(config.Dir(*dir))
	if err != nil {
		fmt.Fprintln(errOut, "cost: "+err.Error())
		return 1
	}
	s, err := led.Summarize()
	if err != nil {
		fmt.Fprintln(errOut, "cost: "+err.Error())
		return 1
	}

	fmt.Fprintf(out, "ledger: %s\n", led.Path())
	fmt.Fprintf(out, "turns:  %d\n", s.Turns)
	fmt.Fprintf(out, "tokens: %d in, %d out (%d total)\n", s.InputTokens, s.OutputTokens, s.InputTokens+s.OutputTokens)
	fmt.Fprintf(out, "spend:  ~$%.4f (estimated)\n", s.USD)
	if len(s.Verdicts) > 0 {
		keys := make([]string, 0, len(s.Verdicts))
		for k := range s.Verdicts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprint(out, "verdicts:")
		for _, k := range keys {
			fmt.Fprintf(out, " %s=%d", k, s.Verdicts[k])
		}
		fmt.Fprintln(out)
	}
	return 0
}
