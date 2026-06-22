package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/config"
)

// noColor disables decorative glyphs (NO_COLOR convention) for portability and
// dumb terminals.
var noColor = os.Getenv("NO_COLOR") != ""

// gl returns the fancy glyph normally, or an ASCII fallback under NO_COLOR.
func gl(fancy, plain string) string {
	if noColor {
		return plain
	}
	return fancy
}

// cmdChat starts an interactive agentic session: type a task, the agent cooks
// (reads/edits files, runs commands) with live activity, then you ask again.
// The conversation and budget persist for the session; a fresh governor scopes
// loop breakers to each task.
func cmdChat(args []string, out, errOut io.Writer) int {
	f, fs := parseRunFlags("chat", args)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if stdinIsPiped() {
		fmt.Fprintln(errOut, "chat is interactive and needs a terminal; use `run`/`exec` for piped input.")
		return 2
	}

	reader := bufio.NewReader(os.Stdin)
	app := &approver{r: reader, out: out}

	a, cfg, cleanup, err := buildAgent(f, app.Approve)
	if err != nil {
		fmt.Fprintln(errOut, "chat: "+err.Error())
		return 1
	}
	defer cleanup()
	a.SetObserver(func(e agent.Event) { printEvent(out, e) })

	s := &session{a: a, r: reader, out: out, dir: f.dir, cfg: cfg, verify: f.verify}
	return s.loop()
}

type session struct {
	a      *agent.Agent
	r      *bufio.Reader
	out    io.Writer
	dir    string
	cfg    config.Config
	verify bool
}

func (s *session) loop() int {
	fmt.Fprintln(s.out, "cliche — interactive agent. Trust kernel on (hard caps + governor).")
	fmt.Fprintln(s.out, "Type a task. Slash commands: /cost  /clear  /context  /verify  /help  /exit")
	for {
		fmt.Fprint(s.out, "\n"+gl("› ", "> "))
		line, err := s.r.ReadString('\n')
		if err != nil { // EOF (Ctrl-D)
			fmt.Fprintln(s.out)
			return 0
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			if s.slash(line) {
				return 0
			}
			continue
		}
		// Install a SIGINT handler only while a task runs, so Ctrl-C aborts the
		// current task (gracefully, structured) but leaves the session alive;
		// Ctrl-C at the idle prompt uses the default behavior (quit).
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		o, runErr := s.a.Run(ctx, line)
		stop()
		if runErr != nil {
			fmt.Fprintln(s.out, "error: "+runErr.Error())
			continue
		}
		s.afterTask(o)
	}
}

func (s *session) afterTask(o agent.Outcome) {
	switch o.Stop {
	case agent.StopCompleted:
		fmt.Fprintf(s.out, "\n%s done (%d turns)\n", gl("✔", "[done]"), o.Turns)
	case agent.StopCancelled:
		fmt.Fprintf(s.out, "\n%s interrupted\n", gl("■", "[x]"))
	case agent.StopBudget:
		fmt.Fprintf(s.out, "\n%s stopped: budget — %s\n", gl("■", "[x]"), o.Reason)
	default:
		fmt.Fprintf(s.out, "\n%s stopped: %s — %s\n", gl("■", "[x]"), o.Stop, o.Reason)
	}
	u := s.a.Usage()
	fmt.Fprintf(s.out, "  session so far: %d tokens, ~$%.4f\n", u.TotalTokens(), u.USD)
	if s.verify && o.Stop == agent.StopCompleted {
		v := autoVerify(s.out, s.dir, s.cfg)
		fmt.Fprintf(s.out, "  verdict: %s\n", v.Status)
	}
}

// slash handles a slash command, returning true if the session should exit.
func (s *session) slash(line string) bool {
	switch strings.Fields(line)[0] {
	case "/exit", "/quit":
		fmt.Fprintln(s.out, "bye.")
		return true
	case "/cost":
		u := s.a.Usage()
		lim := s.a.Limits()
		fmt.Fprintf(s.out, "  session: %d tokens, ~$%.4f", u.TotalTokens(), u.USD)
		if lim.MaxUSD > 0 {
			fmt.Fprintf(s.out, " of ~$%.2f cap", lim.MaxUSD)
		}
		fmt.Fprintln(s.out)
	case "/clear":
		s.a.Reset()
		fmt.Fprintln(s.out, "  context cleared (budget preserved).")
	case "/context":
		est, compactions := s.a.ContextStats()
		fmt.Fprintf(s.out, "  context: ~%d tokens, %d compaction(s) this session\n", est, compactions)
	case "/recover":
		if s.a.RecoverContext() {
			fmt.Fprintln(s.out, "  restored the pre-compaction context.")
		} else {
			fmt.Fprintln(s.out, "  nothing to recover.")
		}
	case "/verify":
		v := autoVerify(s.out, s.dir, s.cfg)
		fmt.Fprintf(s.out, "  verdict: %s\n", v.Status)
	case "/help":
		fmt.Fprintln(s.out, "  /cost — spend so far    /context — context usage   /verify — re-run tests")
		fmt.Fprintln(s.out, "  /clear — reset context  /recover — undo compaction  /exit — quit")
	default:
		fmt.Fprintf(s.out, "  unknown command (try /help)\n")
	}
	return false
}

// printEvent renders one live activity event from the agent loop.
func printEvent(out io.Writer, e agent.Event) {
	switch e.Kind {
	case "text":
		if t := strings.TrimSpace(e.Text); t != "" {
			fmt.Fprintf(out, "\n%s\n", t)
		}
	case "tool_call":
		bullet := gl("●", "*")
		if e.Detail != "" {
			fmt.Fprintf(out, "  %s %s  %s\n", bullet, e.Tool, e.Detail)
		} else {
			fmt.Fprintf(out, "  %s %s\n", bullet, e.Tool)
		}
	case "tool_result":
		if !e.OK { // only surface failures to keep the feed readable
			fmt.Fprintf(out, "    %s %s\n", gl("✗", "x"), e.Detail)
		}
	case "halt":
		fmt.Fprintf(out, "  %s halted: %s\n", gl("■", "!"), e.Detail)
	case "budget":
		fmt.Fprintf(out, "  %s budget: %s\n", gl("■", "!"), e.Detail)
	case "context":
		fmt.Fprintf(out, "  %s context compacted: %s\n", gl("◆", "~"), e.Detail)
	case "warn":
		fmt.Fprintf(out, "  ! %s\n", e.Detail)
	}
}
