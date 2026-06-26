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
	"strings"
	"time"
)

// OpenAICompat is a provider for any OpenAI-compatible Chat Completions API —
// OpenRouter, OpenAI, and most local servers. It supports multi-turn tool
// calling and shares the retry/backoff behavior with the Anthropic backend.
type OpenAICompat struct {
	apiKey       string
	model        string
	baseURL      string
	referer      string // OpenRouter attribution headers (optional)
	title        string
	client       *http.Client
	streamClient *http.Client // no total timeout; ctx governs a long stream
	maxTok       int
	maxRetries   int
	retryBase    time.Duration
	headers      map[string]string // extra request headers (gateway auth, etc.)
}

// SetHeaders adds extra request headers, applied last so they can override the
// defaults (e.g. a Bedrock/Vertex/LiteLLM gateway that uses a different auth
// header). This is how Cliche reaches non-Anthropic-native backends via their
// OpenAI-compatible gateways without a bespoke auth implementation.
func (o *OpenAICompat) SetHeaders(h map[string]string) { o.headers = h }

// setHeaders applies the standard headers plus any custom ones to a request.
func (o *OpenAICompat) setHeaders(req *http.Request, stream bool) {
	req.Header.Set("content-type", "application/json")
	if o.apiKey != "" { // local servers (Ollama, LM Studio) need no auth — don't send an empty Bearer
		req.Header.Set("authorization", "Bearer "+o.apiKey)
	}
	if stream {
		req.Header.Set("accept", "text/event-stream")
	}
	if o.referer != "" {
		req.Header.Set("HTTP-Referer", o.referer)
		req.Header.Set("X-Title", o.title)
	}
	for k, v := range o.headers { // custom headers win (can override authorization)
		req.Header.Set(k, v)
	}
}

// NewOpenAICompat returns a provider for the given Chat Completions endpoint.
func NewOpenAICompat(apiKey, model, baseURL string, maxTokens int) *OpenAICompat {
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &OpenAICompat{
		apiKey:       apiKey,
		model:        model,
		baseURL:      baseURL,
		referer:      "https://github.com/mholovetskyi/cliche",
		title:        "cliche",
		client:       &http.Client{Timeout: 120 * time.Second},
		streamClient: &http.Client{},
		maxTok:       maxTokens,
		maxRetries:   4,
		retryBase:    500 * time.Millisecond,
	}
}

func (o *OpenAICompat) Name() string  { return "openai-compatible" }
func (o *OpenAICompat) Model() string { return o.model }

// ---- wire types ----

type oaiFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // a JSON-encoded string
}

type oaiToolCall struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Function oaiFunc `json:"function"`
}

type oaiMessage struct {
	Role string `json:"role"`
	// Content is a string for plain text, an array of parts for a vision message
	// (text + image_url), or null for an assistant tool-call turn.
	Content    any           `json:"content"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

// oaiContentPart is one element of a multi-part (vision) message content array.
type oaiContentPart struct {
	Type     string       `json:"type"` // "text" | "image_url"
	Text     string       `json:"text,omitempty"`
	ImageURL *oaiImageURL `json:"image_url,omitempty"`
}

type oaiImageURL struct {
	URL string `json:"url"` // a data: URI (data:<media-type>;base64,<data>)
}

type oaiToolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type oaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type oaiRequest struct {
	Model         string            `json:"model"`
	Messages      []oaiMessage      `json:"messages"`
	Tools         []oaiToolDef      `json:"tools,omitempty"`
	MaxTokens     int               `json:"max_tokens,omitempty"`
	Stream        bool              `json:"stream,omitempty"`
	StreamOptions *oaiStreamOptions `json:"stream_options,omitempty"`
}

type oaiResponse struct {
	Choices []struct {
		Message      oaiMessage `json:"message"`
		FinishReason string     `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func strPtr(s string) *string { return &s }

// imageParts builds an OpenAI multi-part (vision) content array: a lead text part
// followed by one image_url part per supported image.
func imageParts(lead string, imgs []Image) []oaiContentPart {
	parts := make([]oaiContentPart, 0, len(imgs)+1)
	if lead != "" {
		parts = append(parts, oaiContentPart{Type: "text", Text: lead})
	}
	for _, img := range imgs {
		if !strings.HasPrefix(img.MediaType, "image/") {
			continue
		}
		uri := "data:" + img.MediaType + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
		parts = append(parts, oaiContentPart{Type: "image_url", ImageURL: &oaiImageURL{URL: uri}})
	}
	return parts
}

func (o *OpenAICompat) buildRequestBody(req Request, stream bool) ([]byte, error) {
	var msgs []oaiMessage
	if req.System != "" {
		msgs = append(msgs, oaiMessage{Role: "system", Content: strPtr(req.System)})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "assistant":
			om := oaiMessage{Role: "assistant"}
			if m.Text != "" {
				om.Content = strPtr(m.Text)
			}
			for _, tc := range m.ToolCalls {
				om.ToolCalls = append(om.ToolCalls, oaiToolCall{
					ID: tc.ID, Type: "function",
					Function: oaiFunc{Name: tc.Name, Arguments: argString(tc)},
				})
			}
			msgs = append(msgs, om)
		default: // user
			if len(m.ToolResults) > 0 {
				for _, tr := range m.ToolResults {
					content := tr.Content
					if content == "" {
						content = "(no output)"
					}
					msgs = append(msgs, oaiMessage{Role: "tool", ToolCallID: tr.ID, Content: strPtr(content)})
				}
				// A tool (e.g. screenshot) can return images. The OpenAI "tool" role
				// can't carry images, so surface them in a following user message — a
				// valid ordering (tool replies, then the user shows what they produced).
				if vp := imageParts("Screenshot from the tool call above:", m.Images); len(vp) > 1 {
					msgs = append(msgs, oaiMessage{Role: "user", Content: vp})
				}
			} else if len(m.Images) > 0 {
				// Vision message: content is an array of text + image_url parts.
				// image_url only supports images, so PDFs/documents are skipped here.
				parts := make([]oaiContentPart, 0, len(m.Images)+1)
				if m.Text != "" {
					parts = append(parts, oaiContentPart{Type: "text", Text: m.Text})
				}
				for _, img := range m.Images {
					if !strings.HasPrefix(img.MediaType, "image/") {
						continue
					}
					uri := "data:" + img.MediaType + ";base64," + base64.StdEncoding.EncodeToString(img.Data)
					parts = append(parts, oaiContentPart{Type: "image_url", ImageURL: &oaiImageURL{URL: uri}})
				}
				if len(parts) == 0 { // all attachments unsupported here → fall back to text
					msgs = append(msgs, oaiMessage{Role: "user", Content: strPtr(m.Text)})
				} else {
					msgs = append(msgs, oaiMessage{Role: "user", Content: parts})
				}
			} else if m.Text != "" {
				msgs = append(msgs, oaiMessage{Role: "user", Content: strPtr(m.Text)})
			}
		}
	}

	var tools []oaiToolDef
	for _, t := range req.Tools {
		var td oaiToolDef
		td.Type = "function"
		td.Function.Name = t.Name
		td.Function.Description = t.Description
		td.Function.Parameters = t.Schema
		tools = append(tools, td)
	}

	maxTok := o.maxTok
	if req.MaxOutputTokens > 0 && req.MaxOutputTokens < maxTok {
		maxTok = req.MaxOutputTokens
	}
	model := o.model
	if req.Model != "" {
		model = req.Model // honor a per-request / in-session model switch
	}
	r := oaiRequest{Model: model, Messages: msgs, Tools: tools, MaxTokens: maxTok}
	if stream {
		r.Stream = true
		r.StreamOptions = &oaiStreamOptions{IncludeUsage: true} // get usage on the final chunk
	}
	return json.Marshal(r)
}

// argString returns the tool-call arguments as the JSON string OpenAI expects.
func argString(tc ToolCall) string {
	if len(tc.Raw) > 0 {
		return string(tc.Raw)
	}
	if len(tc.Args) == 0 {
		return "{}"
	}
	if b, err := json.Marshal(tc.Args); err == nil {
		return string(b)
	}
	return "{}"
}

func parseOpenAIResponse(raw []byte) (Response, error) {
	var p oaiResponse
	if err := json.Unmarshal(raw, &p); err != nil {
		return Response{}, fmt.Errorf("decoding response: %w", err)
	}
	if p.Error != nil {
		return Response{}, fmt.Errorf("api error: %s", p.Error.Message)
	}
	if len(p.Choices) == 0 {
		return Response{}, fmt.Errorf("api returned no choices")
	}
	msg := p.Choices[0].Message
	text := ""
	if s, ok := msg.Content.(string); ok { // responses carry content as a string
		text = s
	}
	var calls []ToolCall
	for _, tc := range msg.ToolCalls {
		args := decodeInput(json.RawMessage(tc.Function.Arguments))
		calls = append(calls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Args:      args,
			Raw:       validRawInput([]byte(tc.Function.Arguments)),
			Signature: signature(tc.Function.Name, args),
		})
	}
	return Response{
		Text:      text,
		ToolCalls: calls,
		Usage:     Usage{InputTokens: p.Usage.PromptTokens, OutputTokens: p.Usage.CompletionTokens},
		Done:      len(calls) == 0,
	}, nil
}

// Complete performs one multi-turn step, retrying transient failures.
func (o *OpenAICompat) Complete(ctx context.Context, req Request) (Response, error) {
	if req.OnDelta != nil {
		return o.completeStream(ctx, req)
	}
	body, err := o.buildRequestBody(req, false)
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
		raw, status, retryAfter, derr := o.doOnce(ctx, body)
		switch {
		case derr != nil:
			if ctx.Err() != nil {
				return Response{}, ctx.Err()
			}
			lastErr = derr
		case status >= 400:
			if !isRetryable(status) {
				if _, perr := parseOpenAIResponse(raw); perr != nil {
					return Response{}, perr
				}
				return Response{}, fmt.Errorf("api returned status %d", status)
			}
			lastErr = fmt.Errorf("api returned retryable status %d", status)
		default:
			return parseOpenAIResponse(raw)
		}
		if attempt >= o.maxRetries {
			return Response{}, lastErr
		}
		delay = backoffDelay(o.retryBase, attempt, retryAfter)
	}
}

// completeStream streams one turn over SSE, emitting text deltas live and
// honoring ctx for token-by-token abort. Retries only before any delta.
func (o *OpenAICompat) completeStream(ctx context.Context, req Request) (Response, error) {
	body, err := o.buildRequestBody(req, true)
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
		resp, emitted, retryable, retryAfter, derr := o.streamOnce(ctx, body, req.OnDelta)
		if derr == nil {
			return resp, nil
		}
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}
		if emitted || !retryable || attempt >= o.maxRetries {
			return Response{}, derr
		}
		delay = backoffDelay(o.retryBase, attempt, retryAfter)
	}
}

func (o *OpenAICompat) streamOnce(ctx context.Context, body []byte, onDelta func(string)) (Response, bool, bool, time.Duration, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL, bytes.NewReader(body))
	if err != nil {
		return Response{}, false, false, 0, err
	}
	o.setHeaders(httpReq, true)
	resp, err := o.streamClient.Do(httpReq)
	if err != nil {
		return Response{}, false, true, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
		ra := parseRetryAfter(resp.Header.Get("Retry-After"))
		if isRetryable(resp.StatusCode) {
			return Response{}, false, true, ra, fmt.Errorf("api returned retryable status %d", resp.StatusCode)
		}
		if _, perr := parseOpenAIResponse(raw); perr != nil {
			return Response{}, false, false, 0, perr
		}
		return Response{}, false, false, 0, fmt.Errorf("api returned status %d", resp.StatusCode)
	}
	return parseOpenAIStream(ctx, resp.Body, onDelta)
}

type oaiToolAccum struct {
	id, name string
	buf      strings.Builder
}

// parseOpenAIStream consumes an OpenAI-compatible SSE stream, accumulating text
// + tool calls + usage and emitting text deltas live. Tool-call arguments arrive
// in fragments keyed by index and are concatenated.
func parseOpenAIStream(ctx context.Context, r io.Reader, onDelta func(string)) (Response, bool, bool, time.Duration, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxBodyBytes)
	var text strings.Builder
	calls := map[int]*oaiToolAccum{}
	var order []int
	usage := Usage{}
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
		if data == "" || data == "[DONE]" {
			if data == "[DONE]" {
				break
			}
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if chunk.Usage != nil {
			usage.InputTokens = chunk.Usage.PromptTokens
			usage.OutputTokens = chunk.Usage.CompletionTokens
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				text.WriteString(ch.Delta.Content)
				if onDelta != nil {
					onDelta(ch.Delta.Content)
					emitted = true
				}
			}
			for _, tc := range ch.Delta.ToolCalls {
				acc := calls[tc.Index]
				if acc == nil {
					acc = &oaiToolAccum{}
					calls[tc.Index] = acc
					order = append(order, tc.Index)
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				acc.buf.WriteString(tc.Function.Arguments)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Response{}, emitted, false, 0, err
	}

	var toolCalls []ToolCall
	for _, idx := range order {
		acc := calls[idx]
		// Normalize empty OR invalid (truncated) input to "{}".
		raw := validRawInput([]byte(acc.buf.String()))
		args := decodeInput(raw)
		toolCalls = append(toolCalls, ToolCall{
			ID: acc.id, Name: acc.name, Args: args,
			Raw:       raw,
			Signature: signature(acc.name, args),
		})
	}
	return Response{Text: text.String(), ToolCalls: toolCalls, Usage: usage, Done: len(toolCalls) == 0}, emitted, false, 0, nil
}

func (o *OpenAICompat) doOnce(ctx context.Context, body []byte) (raw []byte, status int, retryAfter time.Duration, err error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, 0, 0, err
	}
	o.setHeaders(httpReq, false)
	resp, err := o.client.Do(httpReq)
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
