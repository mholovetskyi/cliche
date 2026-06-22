package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Anthropic is the first real BYO-key backend. It supports a full multi-turn
// tool-use loop against the Messages API: it advertises tools, emits tool_use
// blocks, and consumes tool_result blocks fed back by the agent.
type Anthropic struct {
	apiKey string
	model  string
	client *http.Client
	maxTok int
}

// NewAnthropic returns an Anthropic provider. maxTokens bounds the response
// length per turn (a precondition of the dollar-cap guarantee).
func NewAnthropic(apiKey, model string, maxTokens int) *Anthropic {
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &Anthropic{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 120 * time.Second},
		maxTok: maxTokens,
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
	ID    string            `json:"id,omitempty"`
	Name  string            `json:"name,omitempty"`
	Input map[string]string `json:"input,omitempty"`
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
				blocks = append(blocks, contentBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: tc.Args})
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

// Complete performs one multi-turn step against the Anthropic Messages API.
func (a *Anthropic) Complete(ctx context.Context, req Request) (Response, error) {
	body, err := a.buildRequestBody(req)
	if err != nil {
		return Response{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		// Prefer the structured API error in the body; fall back to status.
		if _, perr := parseResponse(raw); perr != nil {
			return Response{}, perr
		}
		return Response{}, fmt.Errorf("anthropic api returned status %d", resp.StatusCode)
	}
	return parseResponse(raw)
}
