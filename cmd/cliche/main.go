// Command cliche is the AI coding agent you can actually leave running.
//
// It wraps any model in a deterministic Trust Kernel: hard token caps with
// estimated dollar caps, a loop circuit-breaker, an append-only cost ledger,
// and a reward-hacking verifier — on by default, open, and auditable.
package main

import (
	"os"

	"github.com/mholovetskyi/cliche/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args, os.Stdout, os.Stderr))
}
