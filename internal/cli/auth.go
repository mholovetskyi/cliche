package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mholovetskyi/cliche/internal/secrets"
	"github.com/mholovetskyi/cliche/internal/style"
)

// cmdAuth manages saved BYO-key credentials so a provider is configured once
// instead of re-exported every shell. With no provider it shows status; with a
// provider it saves a key (from --key, --from-file, or piped stdin) or removes
// one (--remove). Keys are never printed back.
func cmdAuth(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("auth", flag.ContinueOnError)
	key := fs.String("key", "", "the API key value (note: command-line args can leak into shell history)")
	fromFile := fs.String("from-file", "", "read the key from a file")
	remove := fs.Bool("remove", false, "remove the saved key for the provider")

	// The provider is a leading positional (e.g. `auth openrouter --from-file f`).
	// Go's flag parser stops at the first non-flag token, so extract the provider
	// first and parse the remaining flags after it.
	provider := ""
	rest := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		provider = strings.ToLower(args[0])
		rest = args[1:]
	}
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if provider == "" {
		printAuthStatus(out)
		return 0
	}
	if !secrets.Known(provider) {
		fmt.Fprintf(errOut, "auth: unknown provider %q (want anthropic, openrouter, or openai)\n", provider)
		return 2
	}

	if *remove {
		if err := secrets.Remove(provider); err != nil {
			fmt.Fprintln(errOut, "auth: "+err.Error())
			return 1
		}
		fmt.Fprintf(out, "  removed saved %s key.\n", provider)
		return 0
	}

	val, code := readKey(*key, *fromFile, errOut)
	if code != 0 {
		return code
	}
	path, err := secrets.Save(provider, val)
	if err != nil {
		fmt.Fprintln(errOut, "auth: "+err.Error())
		return 1
	}
	fmt.Fprintf(out, "  %s saved %s key to %s\n", style.Red(gl("✔", "+")), style.White(provider), path)
	fmt.Fprintln(out, "  "+style.Gray("stored 0600; the environment variable still overrides it when set."))
	return 0
}

// readKey resolves the key value from the explicit flag, a file, or piped stdin
// (in that order), returning the value or a non-zero exit code with guidance.
func readKey(key, fromFile string, errOut io.Writer) (string, int) {
	switch {
	case strings.TrimSpace(key) != "":
		return strings.TrimSpace(key), 0
	case fromFile != "":
		data, err := os.ReadFile(fromFile)
		if err != nil {
			fmt.Fprintln(errOut, "auth: "+err.Error())
			return "", 1
		}
		return strings.TrimSpace(string(data)), 0
	case stdinIsPiped():
		data, _ := io.ReadAll(os.Stdin)
		if v := strings.TrimSpace(string(data)); v != "" {
			return v, 0
		}
	}
	fmt.Fprintln(errOut, "auth: provide the key with --key <k>, --from-file <path>, or piped on stdin")
	return "", 2
}

// printAuthStatus shows which providers have a key configured and from where —
// never the key itself.
func printAuthStatus(out io.Writer) {
	fmt.Fprintln(out, "\n  "+style.BoldWhite("credentials")+style.Gray("  ·  set a key with `cliche auth <provider>`"))
	for _, p := range supportedProviders {
		if _, source := secrets.Lookup(p); source != "" {
			fmt.Fprintf(out, "  %s %s %s\n", style.Red(gl("✔", "+")), style.White(fmt.Sprintf("%-11s", p)), style.Gray("configured ("+source+")"))
		} else {
			fmt.Fprintf(out, "  %s %s %s\n", style.Gray(gl("·", "-")), style.Gray(fmt.Sprintf("%-11s", p)), style.Gray("not set ("+secrets.EnvVar(p)+")"))
		}
	}
	if path := secrets.CredentialsPath(); path != "" {
		fmt.Fprintln(out, "\n  "+style.Gray("file: "+path))
	}
}
