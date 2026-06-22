package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAICompat is a provider for any OpenAI-compatible Chat Completions API —
// OpenRouter, OpenAI, and most local servers. It supports multi-turn tool
// calling and shares the retry/backoff behavior with the Anthropic backend.
type OpenAICompat struct {
	apiKey     string
	model      string
	baseURL    string
	referer    string // OpenRouter attribution headers (optional)
	title      string
	client     *http.Client
	maxTok     int
	maxRetries int
	retryBase  time.Duration
}

// NewOpenAICompat returns a provider for the given Chat Completions endpoint.
func NewOpenAICompat(apiKey, model, baseURL string, maxTokens int) *OpenAICompat {
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &OpenAICompat{
		apiKey:     apiKey,
		model:      model,
		baseURL:    baseURL,
		referer:    "https://github.com/mholovetskyi/cliche",
		title:      "cliche",
		client:     &http.Client{Timeout: 120 * time.Second},
		maxTok:     maxTokens,
		maxRetries: 4,
		retryBase:  500 * time.Millisecond,
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
	Role       string        `json:"role"`
	Content    *string       `json:"content"` // nullable (assistant tool-call turns)
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type oaiToolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type oaiRequest struct {
	Model     string       `json:"model"`
	Messages  []oaiMessage `json:"messages"`
	Tools     []oaiToolDef `json:"tools,omitempty"`
	MaxTokens int          `json:"max_tokens,omitempty"`
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

func (o *OpenAICompat) buildRequestBody(req Request) ([]byte, error) {
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
	return json.Marshal(oaiRequest{Model: o.model, Messages: msgs, Tools: tools, MaxTokens: maxTok})
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
	if msg.Content != nil {
		text = *msg.Content
	}
	var calls []ToolCall
	for _, tc := range msg.ToolCalls {
		args := decodeInput(json.RawMessage(tc.Function.Arguments))
		calls = append(calls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Args:      args,
			Raw:       json.RawMessage(tc.Function.Arguments),
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
	body, err := o.buildRequestBody(req)
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

func (o *OpenAICompat) doOnce(ctx context.Context, body []byte) (raw []byte, status int, retryAfter time.Duration, err error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, 0, 0, err
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("authorization", "Bearer "+o.apiKey)
	if o.referer != "" {
		httpReq.Header.Set("HTTP-Referer", o.referer)
		httpReq.Header.Set("X-Title", o.title)
	}
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
