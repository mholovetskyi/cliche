package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/config"
	"github.com/mholovetskyi/cliche/internal/orgpolicy"
	"github.com/mholovetskyi/cliche/internal/seal"
	"github.com/mholovetskyi/cliche/internal/secrets"
	"github.com/mholovetskyi/cliche/internal/style"
)

// promptLine reads a single trimmed line for an interactive prompt.
func promptLine(r *bufio.Reader, out io.Writer, msg string) string {
	fmt.Fprint(out, msg)
	line, _ := r.ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

// fetchOrgPolicy GETs the signed policy document from the control plane with the
// subscription bearer token. A 402/401 means the subscription isn't active.
func fetchOrgPolicy(ctx context.Context, oc secrets.OrgConfig) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, oc.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+oc.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	case http.StatusPaymentRequired, http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("subscription not active (HTTP %d) — check your plan or token", resp.StatusCode)
	default:
		return nil, fmt.Errorf("control plane returned HTTP %d", resp.StatusCode)
	}
}

// orgPolicy resolves the effective org policy for a run: nothing configured →
// (zero, false, nil); configured → fetch + verify against the pinned key.
// It FAILS CLOSED: a configured-but-unreachable/unverifiable policy returns an
// error so the caller refuses to run unpoliced.
func orgPolicy() (orgpolicy.Policy, bool, error) {
	oc, ok := secrets.Org()
	if !ok {
		return orgpolicy.Policy{}, false, nil
	}
	pub, err := orgpolicy.ParseKey(oc.Key)
	if err != nil {
		return orgpolicy.Policy{}, false, fmt.Errorf("org policy key: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	data, err := fetchOrgPolicy(ctx, oc)
	if err != nil {
		return orgpolicy.Policy{}, false, fmt.Errorf("org policy: %w", err)
	}
	pol, err := orgpolicy.Load(data, pub)
	if err != nil {
		return orgpolicy.Policy{}, false, fmt.Errorf("org policy: %w", err)
	}
	return pol, true, nil
}

// applyOrgPolicy folds the CLI cap flags into cfg and then, if a control plane
// is connected, fetches + verifies + tighten-only-merges the org policy. It
// fails closed and enforces the flag-level fields (ForbidYolo/ForceSandbox).
// Both the single-agent and swarm builders route through it, so neither path
// can be used to bypass org governance.
func applyOrgPolicy(cfg config.Config, f *runFlags) (config.Config, error) {
	// Fold cap flags FIRST so the policy tightens the EFFECTIVE ceiling — a
	// --max-usd flag must never out-loosen the org's cap.
	if f.maxUSD >= 0 {
		cfg.Budget.MaxUSD = f.maxUSD
	}
	if f.maxTokens >= 0 {
		cfg.Budget.MaxTokens = f.maxTokens
	}
	if f.maxTurns >= 0 {
		cfg.Governor.MaxTurns = f.maxTurns
	}
	pol, ok, err := orgPolicy()
	if err != nil {
		return cfg, fmt.Errorf("%w (refusing to run unpoliced)", err)
	}
	if !ok {
		return cfg, nil
	}
	cfg = orgpolicy.Apply(cfg, pol)
	if pol.ForbidYolo && f.yolo {
		return cfg, fmt.Errorf("org policy forbids --yolo")
	}
	if pol.ForceSandbox {
		f.sandbox = true
	}
	if err := cfg.Validate(); err != nil { // defensive: tightening must keep it valid
		return cfg, fmt.Errorf("org policy produced an invalid config: %w", err)
	}
	return cfg, nil
}

// cmdOrg manages the control-plane connection: `cliche org [login|show|logout]`.
func cmdOrg(args []string, out, errOut io.Writer) int {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "", "status":
		return orgStatus(out)
	case "login", "connect":
		return orgLogin(bufio.NewReader(os.Stdin), out, errOut)
	case "show":
		return orgShow(out, errOut)
	case "logout", "disconnect":
		if err := secrets.DeleteOrg(); err != nil {
			fmt.Fprintln(errOut, "org: "+err.Error())
			return 1
		}
		fmt.Fprintln(out, "  disconnected from the control plane.")
		return 0
	default:
		fmt.Fprintf(errOut, "org: unknown subcommand %q (want login | show | logout)\n", sub)
		return 2
	}
}

func orgStatus(out io.Writer) int {
	oc, ok := secrets.Org()
	if !ok {
		fmt.Fprintln(out, "  "+style.Gray("not connected to a control plane — `cliche org login` to enforce org policy."))
		fmt.Fprintln(out, "  "+style.Gray("teams: see COMMERCIAL.md"))
		return 0
	}
	fmt.Fprintf(out, "  %s connected %s\n", style.Green(gl("✓", "ok")), style.Gray("· "+oc.URL))
	if pub, err := orgpolicy.ParseKey(oc.Key); err == nil {
		fmt.Fprintf(out, "  %s pinned key %s\n", style.Gray("·"), style.Gray(seal.Fingerprint(pub)))
	}
	return 0
}

func orgLogin(r *bufio.Reader, out, errOut io.Writer) int {
	fmt.Fprintln(out, "  "+style.BoldWhite("connect to a Cliche control plane"))
	url := promptLine(r, out, "  policy URL: ")
	key := promptLine(r, out, "  pinned org key (ed25519:…): ")
	tok, err := readSecret(out, r, "  subscription token (hidden): ")
	if err != nil || strings.TrimSpace(tok) == "" {
		fmt.Fprintln(out, "  cancelled.")
		return 1
	}
	oc := secrets.OrgConfig{URL: strings.TrimSpace(url), Token: strings.TrimSpace(tok), Key: strings.TrimSpace(key)}
	if oc.URL == "" || oc.Key == "" {
		fmt.Fprintln(errOut, "org: a policy URL and pinned key are required.")
		return 1
	}
	// Verify the connection works before saving: the key must parse and the
	// control plane must return a policy that verifies against it.
	pub, err := orgpolicy.ParseKey(oc.Key)
	if err != nil {
		fmt.Fprintln(errOut, "org: "+err.Error())
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	data, err := fetchOrgPolicy(ctx, oc)
	if err != nil {
		fmt.Fprintln(errOut, "org: "+err.Error())
		return 1
	}
	if _, err := orgpolicy.Load(data, pub); err != nil {
		fmt.Fprintln(errOut, "org: "+err.Error()+" — wrong pinned key, or an untrusted control plane.")
		return 1
	}
	if err := secrets.SaveOrg(oc); err != nil {
		fmt.Fprintln(errOut, "org: "+err.Error())
		return 1
	}
	fmt.Fprintf(out, "  %s connected — org policy now applies to every run.\n", style.Green(gl("✓", "ok")))
	return 0
}

func orgShow(out, errOut io.Writer) int {
	pol, ok, err := orgPolicy()
	if err != nil {
		fmt.Fprintln(errOut, "org: "+err.Error())
		return 1
	}
	if !ok {
		fmt.Fprintln(out, "  "+style.Gray("not connected — `cliche org login`."))
		return 0
	}
	// Show what the policy tightens against the conservative defaults.
	base := config.Default()
	eff := orgpolicy.Apply(base, pol)
	fmt.Fprintln(out, "  "+style.BoldWhite("org policy in effect:"))
	if len(pol.Deny) > 0 {
		fmt.Fprintf(out, "    deny        %s\n", style.White(strings.Join(pol.Deny, ", ")))
	}
	if len(pol.EgressAllow) > 0 {
		fmt.Fprintf(out, "    egress      %s\n", style.White(strings.Join(pol.EgressAllow, ", ")))
	}
	if eff.Budget.MaxUSD > 0 {
		fmt.Fprintf(out, "    max $       %s\n", style.White(fmt.Sprintf("$%.2f", eff.Budget.MaxUSD)))
	}
	if eff.Budget.MaxTokens > 0 {
		fmt.Fprintf(out, "    max tokens  %s\n", style.White(fmt.Sprintf("%d", eff.Budget.MaxTokens)))
	}
	fmt.Fprintf(out, "    max turns   %s\n", style.White(fmt.Sprintf("%d", eff.Governor.MaxTurns)))
	if pol.ForbidYolo {
		fmt.Fprintln(out, "    "+style.White("--yolo forbidden (approvals always required)"))
	}
	if pol.ForceSandbox {
		fmt.Fprintln(out, "    "+style.White("sandbox forced on"))
	}
	return 0
}
