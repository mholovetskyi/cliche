package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
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
	apiKey       string
	model        string
	client       *http.Client
	streamClient *http.Client // no total timeout; ctx governs a long stream
	maxTok       int
	baseURL      string
	maxRetries   int
	retryBase    time.Duration
}

// NewAnthropic returns an Anthropic provider. maxTokens bounds the response
// length per turn (a precondition of the dollar-cap guarantee).
func NewAnthropic(apiKey, model string, maxTokens int) *Anthropic {
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &Anthropic{
		apiKey:       apiKey,
		model:        model,
		client:       &http.Client{Timeout: 120 * time.Second},
		streamClient: &http.Client{},
		maxTok:       maxTokens,
		baseURL:      "https://api.anthropic.com/v1/messages",
		maxRetries:   4,
		retryBase:    500 * time.Millisecond,
	}
}

func (a *Anthropic) Name() string  { return "anthropic" }
func (a *Anthropic) Model() string { return a.model }

// ---- wire types ----

// cacheControl marks a content block as a prompt-cache breakpoint. The prefix
// up to (and including) it is cached for ~5 minutes; later requests with the
// same prefix read it at ~0.1× the input price.
type cacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// ephemeral is the shared marker placed on cache breakpoints.
var ephemeral = &cacheControl{Type: "ephemeral"}

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
	// image
	Source *imageSource `json:"source,omitempty"`
	// caching
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

// imageSource is a base64-encoded image for a vision content block.
type imageSource struct {
	Type      string `json:"type"` // "base64"
	MediaType string `json:"media_type"`
	Data      string `json:"data"` // base64
}

// sysBlock is a system-prompt text block; carrying cache_control on the last
// one caches the system prompt AND the tools (tools render before system).
type sysBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
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
	System    []sysBlock    `json:"system,omitempty"`
	Tools     []toolDef     `json:"tools,omitempty"`
	Messages  []wireMessage `json:"messages"`
	Stream    bool          `json:"stream,omitempty"`
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
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// buildRequestBody translates a provider-neutral Request into the Anthropic
// wire format. Split out for testability. stream sets the SSE flag.
func (a *Anthropic) buildRequestBody(req Request, stream bool) ([]byte, error) {
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
			} else {
				for _, img := range m.Images {
					blockType := "image"
					if img.MediaType == "application/pdf" {
						blockType = "document" // Anthropic renders PDFs as document blocks
					}
					blocks = append(blocks, contentBlock{Type: blockType, Source: &imageSource{
						Type: "base64", MediaType: img.MediaType, Data: base64.StdEncoding.EncodeToString(img.Data),
					}})
				}
				if m.Text != "" {
					blocks = append(blocks, contentBlock{Type: "text", Text: m.Text})
				}
			}
		}
		if len(blocks) == 0 {
			continue // Anthropic rejects empty content
		}
		msgs = append(msgs, wireMessage{Role: m.Role, Content: blocks})
	}

	model := a.model
	if req.Model != "" {
		model = req.Model // honor a per-request / in-session model switch
	}

	// Prompt caching: one breakpoint on the system block (caches tools+system,
	// the large stable prefix) and one on the last message block (caches the
	// conversation prefix so each later turn reads it instead of re-sending it).
	var system []sysBlock
	if req.System != "" {
		system = []sysBlock{{Type: "text", Text: req.System, CacheControl: ephemeral}}
	}
	if n := len(msgs); n > 0 {
		if c := len(msgs[n-1].Content); c > 0 {
			msgs[n-1].Content[c-1].CacheControl = ephemeral
		}
	}

	return json.Marshal(anthRequest{
		Model:     model,
		MaxTokens: maxTok,
		System:    system,
		Tools:     tools,
		Messages:  msgs,
		Stream:    stream,
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
				Raw:       validRawInput(c.Input),
				Signature: signature(c.Name, args),
			})
		}
	}

	return Response{
		Text:      text,
		ToolCalls: calls,
		Usage: Usage{
			InputTokens:      parsed.Usage.InputTokens,
			OutputTokens:     parsed.Usage.OutputTokens,
			CacheReadTokens:  parsed.Usage.CacheReadInputTokens,
			CacheWriteTokens: parsed.Usage.CacheCreationInputTokens,
		},
		Done: parsed.StopReason != "tool_use",
	}, nil
}

// toolInputJSON returns the tool_use input to send back: the model's original
// JSON if preserved AND valid, else the string args (defaulting to an empty
// object, which the API requires for a no-arg call). The json.Valid check is
// load-bearing: a tool call truncated mid-stream (e.g. a huge write_file that
// hit max_tokens) leaves Raw as invalid JSON, and shipping that back crashes
// the request marshal ("unexpected end of JSON input"). Normalizing here keeps
// the loop alive so the model can simply retry the call.
func toolInputJSON(tc ToolCall) json.RawMessage {
	if len(tc.Raw) > 0 && json.Valid(tc.Raw) {
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

// validRawInput returns a defensive copy of a tool-call's raw input, normalized
// to "{}" when it is empty or not valid JSON — so a malformed/truncated call is
// never stored (and later re-marshaled or persisted) as broken JSON.
func validRawInput(b []byte) json.RawMessage {
	if len(b) > 0 && json.Valid(b) {
		return append(json.RawMessage(nil), b...)
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
	if req.OnDelta != nil {
		return a.completeStream(ctx, req)
	}
	body, err := a.buildRequestBody(req, false)
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

// completeStream runs one turn over the SSE streaming endpoint, emitting text
// deltas live via req.OnDelta and honoring ctx for token-by-token abort. It
// retries transient failures only before any delta has been emitted (a partial
// stream can't be safely replayed).
func (a *Anthropic) completeStream(ctx context.Context, req Request) (Response, error) {
	body, err := a.buildRequestBody(req, true)
	if err != nil {
		return Response{}, err
	}
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
		resp, emitted, retryable, retryAfter, derr := a.streamOnce(ctx, body, req.OnDelta)
		if derr == nil {
			return resp, nil
		}
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}
		if emitted || !retryable || attempt >= a.maxRetries {
			return Response{}, derr
		}
		delay = a.backoff(attempt, retryAfter)
	}
}

func (a *Anthropic) streamOnce(ctx context.Context, body []byte, onDelta func(string)) (Response, bool, bool, time.Duration, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL, bytes.NewReader(body))
	if err != nil {
		return Response{}, false, false, 0, err
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("accept", "text/event-stream")

	resp, err := a.streamClient.Do(httpReq)
	if err != nil {
		return Response{}, false, true, 0, err // network error: retryable before any output
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
		ra := parseRetryAfter(resp.Header.Get("Retry-After"))
		if isRetryable(resp.StatusCode) {
			return Response{}, false, true, ra, fmt.Errorf("anthropic api returned retryable status %d", resp.StatusCode)
		}
		if _, perr := parseResponse(raw); perr != nil {
			return Response{}, false, false, 0, perr // structured API error
		}
		return Response{}, false, false, 0, fmt.Errorf("anthropic api returned status %d", resp.StatusCode)
	}
	return parseAnthropicStream(ctx, resp.Body, onDelta)
}

// streamEvent is the subset of Anthropic SSE event fields we consume.
type streamEvent struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message struct {
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type toolAccum struct {
	id, name string
	buf      strings.Builder
}

// parseAnthropicStream consumes the SSE event stream, accumulating text + tool
// calls + usage, emitting text deltas live, and aborting between events on ctx
// cancellation. Returns (response, emittedAny, retryable, retryAfter, err).
func parseAnthropicStream(ctx context.Context, r io.Reader, onDelta func(string)) (Response, bool, bool, time.Duration, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxBodyBytes)
	var text strings.Builder
	tools := map[int]*toolAccum{}
	var order []int
	usage := Usage{}
	stopReason := ""
	emitted := false

	for sc.Scan() {
		select {
		case <-ctx.Done():
			return Response{}, emitted, false, 0, ctx.Err()
		default:
		}
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[len("data:"):])
		if data == "" {
			continue
		}
		var ev streamEvent
		if json.Unmarshal([]byte(data), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "message_start":
			usage.InputTokens = ev.Message.Usage.InputTokens
			usage.CacheReadTokens = ev.Message.Usage.CacheReadInputTokens
			usage.CacheWriteTokens = ev.Message.Usage.CacheCreationInputTokens
		case "content_block_start":
			if ev.ContentBlock.Type == "tool_use" {
				tools[ev.Index] = &toolAccum{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
				order = append(order, ev.Index)
			}
		case "content_block_delta":
			switch ev.Delta.Type {
			case "text_delta":
				text.WriteString(ev.Delta.Text)
				if onDelta != nil && ev.Delta.Text != "" {
					onDelta(ev.Delta.Text)
					emitted = true
				}
			case "input_json_delta":
				if t := tools[ev.Index]; t != nil {
					t.buf.WriteString(ev.Delta.PartialJSON)
				}
			}
		case "message_delta":
			if ev.Delta.StopReason != "" {
				stopReason = ev.Delta.StopReason
			}
			if ev.Usage.OutputTokens > 0 {
				usage.OutputTokens = ev.Usage.OutputTokens
			}
		case "error":
			return Response{}, emitted, false, 0, fmt.Errorf("anthropic stream error: %s", ev.Error.Message)
		}
	}
	if err := sc.Err(); err != nil {
		return Response{}, emitted, false, 0, err
	}

	var calls []ToolCall
	for _, idx := range order {
		t := tools[idx]
		// Normalize empty OR invalid (e.g. truncated by max_tokens) input to "{}".
		raw := validRawInput([]byte(t.buf.String()))
		args := decodeInput(raw)
		calls = append(calls, ToolCall{
			ID: t.id, Name: t.name, Args: args,
			Raw:       raw,
			Signature: signature(t.name, args),
		})
	}
	return Response{Text: text.String(), ToolCalls: calls, Usage: usage, Done: stopReason != "tool_use"}, emitted, false, 0, nil
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
