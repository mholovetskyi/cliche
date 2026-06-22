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

// Anthropic is the first real BYO-key backend. v0 implements single-shot
// text completion (no tool-use round-trips yet) so the Trust Kernel can wrap
// a real model end-to-end. Multi-turn tool use is the immediate next step
// (see ROADMAP) and is intentionally out of v0 scope.
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

type anthReqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	System    string           `json:"system,omitempty"`
	Messages  []anthReqMessage `json:"messages"`
}

type anthResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete performs a single-shot completion against the Anthropic Messages
// API. It returns Done=true: v0 does not loop a real model through tools.
func (a *Anthropic) Complete(ctx context.Context, req Request) (Response, error) {
	msgs := make([]anthReqMessage, 0, len(req.Messages)+1)
	for _, m := range req.Messages {
		msgs = append(msgs, anthReqMessage{Role: m.Role, Content: m.Content})
	}
	msgs = append(msgs, anthReqMessage{Role: "user", Content: req.Prompt})

	// Bound output by the remaining token budget when the agent provides one,
	// so a turn cannot blow past the hard token cap.
	maxTok := a.maxTok
	if req.MaxOutputTokens > 0 && req.MaxOutputTokens < maxTok {
		maxTok = req.MaxOutputTokens
	}

	body, err := json.Marshal(anthRequest{
		Model:     a.model,
		MaxTokens: maxTok,
		System:    req.System,
		Messages:  msgs,
	})
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

	var parsed anthResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Response{}, fmt.Errorf("decoding response (status %d): %w", resp.StatusCode, err)
	}
	if parsed.Error != nil {
		return Response{}, fmt.Errorf("anthropic api error: %s: %s", parsed.Error.Type, parsed.Error.Message)
	}
	if resp.StatusCode >= 400 {
		return Response{}, fmt.Errorf("anthropic api returned status %d", resp.StatusCode)
	}

	var text string
	for _, c := range parsed.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}
	return Response{
		Text:  text,
		Done:  true,
		Usage: Usage{InputTokens: parsed.Usage.InputTokens, OutputTokens: parsed.Usage.OutputTokens},
	}, nil
}
