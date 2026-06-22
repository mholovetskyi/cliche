package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxBodyBytes = 16 << 20 // 16 MiB cap on response bodies

// Anthropic is the first real BYO-key backend. It supports a full multi-turn
// tool-use loop against the Messages API: it advertises tools, emits tool_use
// blocks, and consumes tool_result blocks fed back by the agent. Transient
// failures (429/5xx/network) are retried with backoff.
type Anthropic struct {
	apiKey     string
	model      string
	client     *http.Client
	maxTok     int
	baseURL    string
	maxRetries int
	retryBase  time.Duration
}

// NewAnthropic returns an Anthropic provider. maxTokens bounds the response
// length per turn (a precondition of the dollar-cap guarantee).
func NewAnthropic(apiKey, model string, maxTokens int) *Anthropic {
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &Anthropic{
		apiKey:     apiKey,
		model:      model,
		client:     &http.Client{Timeout: 120 * time.Second},
		maxTok:     maxTokens,
		baseURL:    "https://api.anthropic.com/v1/messages",
		maxRetries: 4,
		retryBase:  500 * time.Millisecond,
	}
}

func (a *Anthropic) Name() string  { return "anthropic" }
func (a *Anthropic) Model() string { return a.model }

// ---- wire types ----

type contentBlock struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type wireMessage struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system,omitempty"`
	Tools     []toolDef     `json:"tools,omitempty"`
	Messages  []wireMessage `json:"messages"`
}

type anthResponse struct {
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// buildRequestBody translates a provider-neutral Request into the Anthropic
// wire format. Split out for testability.
func (a *Anthropic) buildRequestBody(req Request) ([]byte, error) {
	maxTok := a.maxTok
	if req.MaxOutputTokens > 0 && req.MaxOutputTokens < maxTok {
		maxTok = req.MaxOutputTokens
	}

	var tools []toolDef
	for _, t := range req.Tools {
		tools = append(tools, toolDef{Name: t.Name, Description: t.Description, InputSchema: t.Schema})
	}

	var msgs []wireMessage
	for _, m := range req.Messages {
		var blocks []contentBlock
		switch m.Role {
		case "assistant":
			if m.Text != "" {
				blocks = append(blocks, contentBlock{Type: "text", Text: m.Text})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, contentBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: toolInputJSON(tc)})
			}
		default: // user
			if len(m.ToolResults) > 0 {
				for _, tr := range m.ToolResults {
					content := tr.Content
					if content == "" {
						content = "(no output)"
					}
					blocks = append(blocks, contentBlock{Type: "tool_result", ToolUseID: tr.ID, Content: content, IsError: tr.IsError})
				}
			} else if m.Text != "" {
				blocks = append(blocks, contentBlock{Type: "text", Text: m.Text})
			}
		}
		if len(blocks) == 0 {
			continue // Anthropic rejects empty content
		}
		msgs = append(msgs, wireMessage{Role: m.Role, Content: blocks})
	}

	return json.Marshal(anthRequest{
		Model:     a.model,
		MaxTokens: maxTok,
		System:    req.System,
		Tools:     tools,
		Messages:  msgs,
	})
}

// parseResponse turns a raw Messages API body into a provider Response.
func parseResponse(raw []byte) (Response, error) {
	var parsed anthResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Response{}, fmt.Errorf("decoding response: %w", err)
	}
	if parsed.Error != nil {
		return Response{}, fmt.Errorf("anthropic api error: %s: %s", parsed.Error.Type, parsed.Error.Message)
	}

	var text string
	var calls []ToolCall
	for _, c := range parsed.Content {
		switch c.Type {
		case "text":
			text += c.Text
		case "tool_use":
			args := decodeInput(c.Input)
			calls = append(calls, ToolCall{
				ID:        c.ID,
				Name:      c.Name,
				Args:      args,
				Raw:       append(json.RawMessage(nil), c.Input...),
				Signature: signature(c.Name, args),
			})
		}
	}

	return Response{
		Text:      text,
		ToolCalls: calls,
		Usage:     Usage{InputTokens: parsed.Usage.InputTokens, OutputTokens: parsed.Usage.OutputTokens},
		Done:      parsed.StopReason != "tool_use",
	}, nil
}

// toolInputJSON returns the tool_use input to send back: the model's original
// JSON if preserved, else the string args (defaulting to an empty object, which
// the API requires for a no-arg call).
func toolInputJSON(tc ToolCall) json.RawMessage {
	if len(tc.Raw) > 0 {
		return tc.Raw
	}
	if len(tc.Args) == 0 {
		return json.RawMessage("{}")
	}
	if b, err := json.Marshal(tc.Args); err == nil {
		return b
	}
	return json.RawMessage("{}")
}

// decodeInput converts an arbitrary JSON tool input object into string args.
func decodeInput(raw json.RawMessage) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return out
	}
	for k, v := range obj {
		if s, ok := v.(string); ok {
			out[k] = s
			continue
		}
		b, _ := json.Marshal(v)
		out[k] = string(b)
	}
	return out
}

// signature builds a stable, order-independent description of a tool call for
// the Governor's repetition detector.
func signature(name string, args map[string]string) string {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(name)
	for _, k := range keys {
		b.WriteString(";")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(args[k])
	}
	return b.String()
}

// Complete performs one multi-turn step against the Anthropic Messages API,
// retrying transient failures (429/5xx/network) with backoff, honoring
// Retry-After, and respecting context cancellation between attempts.
func (a *Anthropic) Complete(ctx context.Context, req Request) (Response, error) {
	body, err := a.buildRequestBody(req)
	if err != nil {
		return Response{}, err
	}

	var lastErr error
	var delay time.Duration
	for attempt := 0; ; attempt++ {
		if delay > 0 {
			t := time.NewTimer(delay)
			select {
			case <-t.C:
			case <-ctx.Done():
				t.Stop()
				return Response{}, ctx.Err()
			}
		}

		raw, status, retryAfter, derr := a.doOnce(ctx, body)
		switch {
		case derr != nil:
			if ctx.Err() != nil {
				return Response{}, ctx.Err()
			}
			lastErr = derr
		case status >= 400:
			if !isRetryable(status) {
				if _, perr := parseResponse(raw); perr != nil {
					return Response{}, perr // structured API error
				}
				return Response{}, fmt.Errorf("anthropic api returned status %d", status)
			}
			lastErr = fmt.Errorf("anthropic api returned retryable status %d", status)
		default:
			return parseResponse(raw)
		}

		if attempt >= a.maxRetries {
			return Response{}, lastErr
		}
		delay = a.backoff(attempt, retryAfter)
	}
}

func (a *Anthropic) doOnce(ctx context.Context, body []byte) (raw []byte, status int, retryAfter time.Duration, err error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, 0, 0, err
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, 0, 0, err
	}
	defer resp.Body.Close()
	raw, rerr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
	if rerr != nil {
		return nil, resp.StatusCode, retryAfter, rerr
	}
	return raw, resp.StatusCode, retryAfter, nil
}

func isRetryable(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, 529:
		return true
	default:
		return false
	}
}

// backoff returns the delay before the next attempt: Retry-After if present,
// else capped exponential backoff.
func (a *Anthropic) backoff(attempt int, retryAfter time.Duration) time.Duration {
	return backoffDelay(a.retryBase, attempt, retryAfter)
}

// backoffDelay is the shared backoff: Retry-After wins, else capped exponential.
func backoffDelay(base time.Duration, attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}
	d := base * time.Duration(int64(1)<<uint(attempt))
	if max := 30 * time.Second; d > max {
		d = max
	}
	return d
}

func parseRetryAfter(h string) time.Duration {
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}
