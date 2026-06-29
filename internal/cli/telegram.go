package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/mholovetskyi/cliche/internal/agent"
	"github.com/mholovetskyi/cliche/internal/cron"
	"github.com/mholovetskyi/cliche/internal/provider"
	"github.com/mholovetskyi/cliche/internal/telegram"
)

// cmdTelegram drives Cliché from a Telegram chat: send a prompt, get the result
// back — from your phone, no app needed. SECURITY: it only ever runs prompts from
// the single authorized owner chat, every run goes through the Trust Kernel (budget
// cap, governor, deny rules), and a rolling daily ceiling bounds total spend. The
// bot token + owner chat id come from the environment, never code.
func cmdTelegram(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("telegram", flag.ContinueOnError)
	dir := fs.String("dir", ".", "project root the agent works in")
	tokenFlag := fs.String("token", "", "bot token (or set CLICHE_TELEGRAM_TOKEN)")
	chatFlag := fs.Int64("chat", 0, "authorized owner chat id (or set CLICHE_TELEGRAM_CHAT)")
	maxDaily := fs.Float64("max-daily-usd", 10.0, "rolling 24h spend ceiling across all chat runs (0 = unlimited)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	token := *tokenFlag
	if token == "" {
		token = os.Getenv("CLICHE_TELEGRAM_TOKEN")
	}
	if token == "" {
		fmt.Fprintln(errOut, "telegram: no bot token. Create a bot with @BotFather in Telegram, then:")
		fmt.Fprintln(errOut, "  export CLICHE_TELEGRAM_TOKEN=<token>   (or pass --token)")
		fmt.Fprintln(errOut, "  cliche telegram                        # message the bot once — it replies with your chat id")
		fmt.Fprintln(errOut, "  export CLICHE_TELEGRAM_CHAT=<that id>   # then restart to authorize your chat")
		return 2
	}
	owner := *chatFlag
	if owner == 0 {
		owner, _ = strconv.ParseInt(strings.TrimSpace(os.Getenv("CLICHE_TELEGRAM_CHAT")), 10, 64)
	}

	client := telegram.New(token)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if owner == 0 {
		fmt.Fprintln(out, "Telegram bot online in SETUP mode — message it and it replies with your chat id.")
		fmt.Fprintln(out, "Set CLICHE_TELEGRAM_CHAT=<that id> and restart to authorize agent runs. Ctrl-C to stop.")
	} else {
		fmt.Fprintf(out, "Telegram bot online for %s — owner chat %d ONLY. Each run is Trust-Kernel-bounded; daily ceiling $%.2f. Ctrl-C to stop.\n", *dir, owner, *maxDaily)
		_ = client.SendMessage(ctx, owner, "Cliché is online. Send a prompt — every run is bounded by the Trust Kernel. /stop to halt.")
	}

	offset := 0
	for {
		updates, err := client.GetUpdates(ctx, offset, 30)
		if ctx.Err() != nil {
			fmt.Fprintln(out, "telegram bot stopped.")
			return 0
		}
		if err != nil { // transient (network / API) — back off and retry
			fmt.Fprintf(errOut, "telegram: %v\n", err)
			select {
			case <-ctx.Done():
				return 0
			case <-time.After(5 * time.Second):
			}
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil {
				continue
			}
			chat := u.Message.Chat.ID
			text := strings.TrimSpace(u.Message.Text)
			if text == "" {
				continue
			}
			if owner == 0 {
				_ = client.SendMessage(ctx, chat, fmt.Sprintf("Your chat id is %d.\nSet CLICHE_TELEGRAM_CHAT=%d and restart Cliché to authorize this chat.", chat, chat))
				continue
			}
			if chat != owner {
				continue // SECURITY: only the owner can drive the agent
			}
			if text == "/stop" {
				_ = client.SendMessage(ctx, owner, "stopping.")
				return 0
			}
			if *maxDaily > 0 && cron.SpentLast24h(*dir) >= *maxDaily {
				_ = client.SendMessage(ctx, owner, fmt.Sprintf("daily ceiling $%.2f reached — try again once the 24h window frees.", *maxDaily))
				continue
			}
			_ = client.SendMessage(ctx, owner, "working…")
			fmt.Fprintf(out, "[%s] chat run: %s\n", time.Now().Format("15:04:05"), cronClip(text, 60))
			o, reply := runForChat(ctx, *dir, text)
			cron.RecordSpend(*dir, o.Usage.USD)
			_ = client.SendMessage(ctx, owner, reply)
		}
	}
}

// runForChat runs one chat prompt headlessly through the agent + Trust Kernel and
// returns the assistant's final reply (with a one-line outcome footer).
func runForChat(ctx context.Context, dir, prompt string) (agent.Outcome, string) {
	f := &runFlags{dir: dir, maxUSD: -1, maxTokens: -1, maxTurns: -1, yolo: true}
	a, _, _, cleanup, err := buildAgent(f, nil, true)
	if err != nil {
		return agent.Outcome{Stop: "error"}, "couldn't start: " + err.Error()
	}
	defer cleanup()
	fctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	o, runErr := a.Run(fctx, prompt)
	if runErr != nil {
		return o, "error: " + runErr.Error()
	}
	reply := lastAssistantText(a.Transcript())
	footer := fmt.Sprintf("— %s · %d turns · $%.4f", o.Stop, o.Turns, o.Usage.USD)
	if strings.TrimSpace(reply) == "" {
		return o, "done " + footer
	}
	return o, reply + "\n\n" + footer
}

func lastAssistantText(msgs []provider.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" && strings.TrimSpace(msgs[i].Text) != "" {
			return msgs[i].Text
		}
	}
	return ""
}
