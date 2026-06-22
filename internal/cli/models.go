package cli

import (
	"fmt"
	"io"

	"github.com/mholovetskyi/cliche/internal/pricing"
	"github.com/mholovetskyi/cliche/internal/style"
)

// cmdModels prints the maintained price table that turns the hard token cap
// into an estimated dollar cap. Showing it is on-brand: the dollar figure is an
// estimate, and the user should be able to see exactly what it rests on.
func cmdModels(_ []string, out, _ io.Writer) int {
	fmt.Fprintln(out, "\n  "+style.BoldWhite("model prices")+style.Gray("  ·  USD per 1M tokens  ·  input / output"))
	fmt.Fprintln(out, "  "+style.Gray("the token cap is the hard guarantee; dollars are estimated from this table."))
	fmt.Fprintln(out, "  "+style.Gray("override with real contracted rates per model in .cliche/config.json."))
	fmt.Fprintln(out)
	for _, e := range pricing.Models() {
		fmt.Fprintf(out, "  %s %s\n", style.White(fmt.Sprintf("%-32s", e.Model)),
			style.Gray(fmt.Sprintf("%8.2f / %8.2f", e.Price.InputPerM, e.Price.OutputPerM)))
	}
	fb := pricing.Fallback()
	fmt.Fprintf(out, "  %s %s  %s\n", style.White(fmt.Sprintf("%-32s", "unknown model (fallback)")),
		style.Gray(fmt.Sprintf("%8.2f / %8.2f", fb.InputPerM, fb.OutputPerM)),
		style.Red("deliberately high — an unknown model can't defeat the dollar cap"))
	return 0
}
