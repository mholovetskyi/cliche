package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/ledger"
	"github.com/mholovetskyi/cliche/internal/verifier"
)

// cmdVerify is the keystone command: it independently re-runs the project's
// tests and combines that with static reward-hacking detectors to reach a
// verdict (verified / unverified / flagged). It is composable in CI:
//
//	git diff | cliche verify --claim-pass
func cmdVerify(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project root")
	diffPath := fs.String("diff", "", "path to a unified diff to inspect, or '-' for stdin (default: stdin if piped)")
	testCmd := fs.String("test-cmd", "", "override the test command")
	noTests := fs.Bool("no-tests", false, "skip the test re-run (static checks only)")
	claimPass := fs.Bool("claim-pass", false, "treat the change as claiming tests pass, and verify that claim")
	strict := fs.Bool("strict", false, "exit non-zero when the result is unverified")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Load the diff. "-" or a piped stdin reads from stdin; a path reads a file.
	diff := ""
	switch {
	case *diffPath == "-":
		if data, _ := io.ReadAll(os.Stdin); len(data) > 0 {
			diff = string(data)
		}
	case *diffPath != "":
		data, err := os.ReadFile(*diffPath)
		if err != nil {
			fmt.Fprintln(errOut, "verify: "+err.Error())
			return 1
		}
		diff = string(data)
	case stdinIsPiped():
		if data, _ := io.ReadAll(os.Stdin); len(data) > 0 {
			diff = string(data)
		}
	}

	// Resolve the test command: flag > config > AGENTS.md/auto-detect.
	cfg, _ := config.Load(*dir)
	cmd := *testCmd
	if cmd == "" {
		cmd = cfg.Verify.TestCommand
	}
	if cmd == "" {
		if c, ok := verifier.DiscoverTestCommand(*dir); ok {
			cmd = c
		}
	}

	var tr verifier.TestResult
	if !*noTests && cmd != "" {
		fmt.Fprintf(out, "re-running tests: %s\n", cmd)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		tr = verifier.RunTests(ctx, *dir, cmd)
		cancel()
	} else if !*noTests && cmd == "" {
		fmt.Fprintln(out, "no test command found (configure verify.test_command or pass --test-cmd)")
	}

	v := verifier.VerifyClaim(diff, *claimPass, tr)

	if tr.Ran {
		status := "FAILED"
		if tr.Passed {
			status = "passed"
		}
		fmt.Fprintf(out, "tests: %s\n", status)
	}
	fmt.Fprintf(out, "verdict: %s\n", v.Status)
	for _, f := range v.Findings {
		fmt.Fprintf(out, "  • [%s] %s\n", f.Rule, f.Detail)
	}

	if led, err := ledger.Open(config.Dir(*dir)); err == nil {
		_ = led.Append(ledger.Entry{Event: ledger.EventVerdict, Verdict: v.Status, Detail: cmd})
	}

	switch v.Status {
	case verifier.StatusVerified:
		return 0
	case verifier.StatusFlagged:
		return 5
	default: // unverified
		if *strict {
			return 2
		}
		return 0
	}
}

// autoVerify re-runs the project's tests after an agent completes a task and
// returns a verdict (used by `run`/`exec --verify` and the chat /verify slash
// command). Progress is written to out; the verdict is recorded to the ledger.
func autoVerify(out io.Writer, dir string, cfg config.Config) verifier.Verdict {
	cmd := cfg.Verify.TestCommand
	if cmd == "" {
		if c, ok := verifier.DiscoverTestCommand(dir); ok {
			cmd = c
		}
	}
	if cmd == "" {
		fmt.Fprintln(out, "  verify: no test command found; skipping")
		return verifier.Verdict{Status: verifier.StatusUnverified}
	}
	fmt.Fprintf(out, "  verify: re-running tests: %s\n", cmd)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	tr := verifier.RunTests(ctx, dir, cmd)
	cancel()
	// The agent made no explicit "tests pass" claim here (auto-verify), so a
	// failing run is reported as tests_failed, not a false-claim.
	v := verifier.VerifyClaim("", false, tr)
	if led, err := ledger.Open(config.Dir(dir)); err == nil {
		_ = led.Append(ledger.Entry{Event: ledger.EventVerdict, Verdict: v.Status, Detail: "auto-verify: " + cmd})
	}
	return v
}
