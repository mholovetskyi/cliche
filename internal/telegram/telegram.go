// Package telegram is a tiny, zero-dependency Telegram Bot API client — just
// long-poll getUpdates + sendMessage over net/http, enough to drive Cliché from a
// chat. No third-party SDK, so the CLI stays dependency-free.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to one bot (identified by its token).
type Client struct {
	token string
	api   string // base URL, overridable in tests
}

// New returns a client for the given bot token.
func New(token string) *Client { return &Client{token: token, api: "https://api.telegram.org"} }

// Chat / Message / Update mirror the slice of the Bot API we use.
type Chat struct {
	ID int64 `json:"id"`
}
type Message struct {
	MessageID int    `json:"message_id"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}
type Update struct {
	UpdateID int      `json:"update_id"`
	Message  *Message `json:"message"`
}

// GetUpdates long-polls for new updates after `offset`, blocking up to
// timeoutSec on the server side.
func (c *Client) GetUpdates(ctx context.Context, offset, timeoutSec int) ([]Update, error) {
	url := fmt.Sprintf("%s/bot%s/getUpdates?timeout=%d&offset=%d", c.api, c.token, timeoutSec, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	cl := &http.Client{Timeout: time.Duration(timeoutSec+15) * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		OK          bool     `json:"ok"`
		Result      []Update `json:"result"`
		Description string   `json:"description"`
		ErrorCode   int      `json:"error_code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("telegram getUpdates: %s (code %d)", out.Description, out.ErrorCode)
	}
	return out.Result, nil
}

// SendMessage posts a text message to a chat (truncated to Telegram's limit).
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	if len(text) > 4000 {
		text = text[:4000] + "…"
	}
	if text == "" {
		text = "(no output)"
	}
	body, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	url := fmt.Sprintf("%s/bot%s/sendMessage", c.api, c.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	cl := &http.Client{Timeout: 30 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram sendMessage: %s", resp.Status)
	}
	return nil
}
