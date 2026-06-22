package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/secrets"
	"github.com/mholovetskyi/cliche/internal/style"
)

// validateKey is indirected so tests can stub the network check.
var validateKey = provider.ValidateKey

// loginChoice is one selectable provider in the login wizard.
type loginChoice struct {
	key, label, where string
}

var loginChoices = []loginChoice{
	{"anthropic", "Anthropic", "console.anthropic.com → API keys"},
	{"openrouter", "OpenRouter", "openrouter.ai/keys · one key, many models"},
	{"openai", "OpenAI", "platform.openai.com/api-keys"},
}

// cmdLogin runs the interactive setup wizard. In a non-interactive context it
// points the user at the scriptable `cliche auth` instead of hanging.
func cmdLogin(_ []string, out, errOut io.Writer) int {
	if stdinIsPiped() {
		fmt.Fprintln(errOut, "login is interactive — in scripts use: cliche auth <provider> --from-file <path> (or --key / stdin)")
		return 2
	}
	return runLogin(bufio.NewReader(os.Stdin), out)
}

// runLogin drives the wizard against the given reader: pick a provider, paste a
// key (hidden), verify it works with a token-free API ping, then save it.
func runLogin(r *bufio.Reader, out io.Writer) int {
	fmt.Fprintln(out, "\n  "+wordmark()+style.Gray("  let's connect a model provider"))
	fmt.Fprintln(out, "  "+style.Gray("Cliche is BYO-key — your key is stored locally (0600), never sent anywhere but the provider.\n"))

	for i, c := range loginChoices {
		mark := " "
		if _, src := secrets.Lookup(c.key); src != "" {
			mark = gl("✓", "*")
		}
		fmt.Fprintf(out, "    %s %d) %s  %s\n", style.Red(mark), i+1, style.White(c.label), style.Gray(c.where))
	}

	choice, ok := readChoice(r, out)
	if !ok {
		fmt.Fprintln(out, "  cancelled.")
		return 1
	}
	c := loginChoices[choice]

	for attempt := 0; attempt < 3; attempt++ {
		key, err := readSecret(out, r, fmt.Sprintf("\n  paste your %s API key (hidden): ", c.label))
		if err != nil || key == "" {
			fmt.Fprintln(out, "  cancelled.")
			return 1
		}
		fmt.Fprint(out, "  checking… ")
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		verr := validateKey(ctx, c.key, key, "")
		cancel()

		switch {
		case verr == nil:
			return saveAndFinish(out, c, key, true)
		case errors.Is(verr, provider.ErrUnauthorized):
			fmt.Fprintln(out, style.Red(gl("✗", "x"))+" that key was rejected — check it and try again.")
			continue
		default:
			fmt.Fprintf(out, "%s couldn't verify (%s).\n", style.Red(gl("⚠", "!")), verr.Error())
			if yesNo(r, out, "  save it anyway? [y/N] ") {
				return saveAndFinish(out, c, key, false)
			}
			return 1
		}
	}
	fmt.Fprintln(out, "  too many attempts — run `cliche login` again when you have the key.")
	return 1
}

func saveAndFinish(out io.Writer, c loginChoice, key string, verified bool) int {
	path, err := secrets.Save(c.key, key)
	if err != nil {
		fmt.Fprintln(out, "  save failed: "+err.Error())
		return 1
	}
	if verified {
		fmt.Fprintln(out, style.Red(gl("✓", "ok"))+" "+style.White("key works")+" — saved.")
	} else {
		fmt.Fprintln(out, "  saved (unverified).")
	}
	fmt.Fprintln(out, "  "+style.Gray("stored 0600 at "+path))
	fmt.Fprintln(out, "\n  "+style.White("you're set.")+style.Gray("  start with `cliche chat`, or one-shot `cliche run \"…\"`."))
	return 0
}

// readChoice reads a provider selection (a number 1-N or a provider name),
// re-prompting on invalid input.
func readChoice(r *bufio.Reader, out io.Writer) (int, bool) {
	for tries := 0; tries < 3; tries++ {
		fmt.Fprintf(out, "\n  choose a provider [1-%d]: ", len(loginChoices))
		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			return 0, false
		}
		s := strings.ToLower(strings.TrimSpace(line))
		if n, perr := strconv.Atoi(s); perr == nil && n >= 1 && n <= len(loginChoices) {
			return n - 1, true
		}
		for i, c := range loginChoices {
			if s == c.key {
				return i, true
			}
		}
		fmt.Fprintln(out, "  please enter a number from the list.")
	}
	return 0, false
}

// readSecret prompts for and reads a line with terminal echo disabled when
// possible (falling back to visible input, with a note, when it isn't).
func readSecret(out io.Writer, r *bufio.Reader, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	var line string
	var err error
	if withEchoDisabled(func() { line, err = r.ReadString('\n') }) {
		fmt.Fprintln(out) // the Enter keystroke wasn't echoed; move to a new line
	} else {
		line, err = r.ReadString('\n')
	}
	return strings.TrimSpace(line), err
}

func yesNo(r *bufio.Reader, out io.Writer, prompt string) bool {
	fmt.Fprint(out, prompt)
	line, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}
