package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/style"
)

// cmdAudit verifies the tamper-evident hash chain of the project's audit ledger.
// Exit 0 = intact, 5 = tampering detected, 1 = error.
func cmdAudit(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	led, err := ledger.Open(config.Dir(*dir))
	if err != nil {
		fmt.Fprintln(errOut, "audit: "+err.Error())
		return 1
	}
	rep, err := led.Verify()
	if err != nil {
		fmt.Fprintln(errOut, "audit: "+err.Error())
		return 1
	}
	renderAudit(out, rep, led.Path())
	if !rep.OK {
		return 5
	}
	return 0
}

func renderAudit(out io.Writer, r ledger.Report, path string) {
	fmt.Fprintf(out, "\n  %s %s\n", style.BoldWhite("audit"), style.Gray("· "+path))
	if r.Entries == 0 {
		fmt.Fprintln(out, "  "+style.Gray("ledger is empty — nothing to verify"))
		return
	}
	if r.OK {
		fmt.Fprintf(out, "  %s %s\n", style.Green(gl("✓", "ok")),
			style.White(fmt.Sprintf("chain intact · %d entries verified", r.Verified)))
		if r.Legacy > 0 {
			noun := "entries"
			if r.Legacy == 1 {
				noun = "entry"
			}
			fmt.Fprintf(out, "  %s\n", style.Gray(fmt.Sprintf("%d legacy %s predate the hash chain (unverifiable)", r.Legacy, noun)))
		}
		fmt.Fprintln(out, "  "+style.Gray("no entry was altered, deleted, reordered, or inserted"))
		return
	}
	fmt.Fprintf(out, "  %s %s\n", style.Red(gl("✗", "x")),
		style.Red(fmt.Sprintf("TAMPER DETECTED at entry %d", r.BrokenAt)))
	fmt.Fprintln(out, "  "+style.Gray(r.Reason))
}
