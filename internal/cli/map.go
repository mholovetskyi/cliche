package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/mholovetskyi/cliche/internal/repomap"
	"github.com/mholovetskyi/cliche/internal/style"
)

// cmdMap prints the project repo map: the structural overview the agent is given
// at startup, useful to inspect on its own.
func cmdMap(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("map", flag.ContinueOnError)
	fs.SetOutput(errOut)
	dir := fs.String("dir", ".", "project directory to map")
	full := fs.Bool("full", false, "do not bound the map size")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	limit := repoMapBudget
	if *full {
		limit = 1 << 30
	}
	m, err := repomap.Build(*dir, limit)
	if err != nil {
		fmt.Fprintln(errOut, "map: "+err.Error())
		return 1
	}
	if m == "" {
		fmt.Fprintln(out, style.Gray("  (no source files found)"))
		return 0
	}
	fmt.Fprintln(out, m)
	return 0
}
