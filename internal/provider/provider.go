// Package provider defines the model-agnostic interface every backend
// implements. Cliche is BYO-key and provider-neutral by design: the Trust
// Kernel wraps whatever model you bring. The mock provider powers tests and
// the offline demo; the anthropic provider is the first real backend and
// supports a full multi-turn tool-use loop.
package provider

import (
	"context"
	"encoding/json"
)

// ToolSpec describes a tool the model may call. Schema is a JSON Schema object
// for the tool's input.
type ToolSpec struct {
	Name        string
	Description string
	Schema      map[string]any
}

// ToolCall is a request from the model to run a tool.
type ToolCall struct {
	ID   string            `json:"id"`
	Name string            `json:"name"`
	Args map[string]string `json:"args"`
	// Raw is the model's original tool input JSON, preserved verbatim so the
	// echoed assistant turn round-trips non-string args (numbers, booleans,
	// nested objects) without lossy stringification.
	Raw json.RawMessage `json:"-"`
	// Signature is a stable description of the call used by the Governor for
	// repetition detection. Two semantically-identical calls must share a
	// signature.
	Signature string `json:"signature"`
}

// ToolResult is the outcome of running a ToolCall, fed back to the model.
type ToolResult struct {
	ID      string
	Content string
	IsError bool
}

// Usage reports token consumption for a single completion.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Message is one turn of the transcript in a provider-neutral form. An
// assistant message carries Text and/or ToolCalls; a user message carries Text
// or ToolResults.
type Message struct {
	Role        string
	Text        string
	ToolCalls   []ToolCall
	ToolResults []ToolResult
}

// Request is a single completion request.
type Request struct {
	System          string
	Model           string
	Messages        []Message
	Tools           []ToolSpec
	MaxOutputTokens int // when > 0, bounds output so a turn can't overshoot the cap
}

// Response is a single completion result.
type Response struct {
	Text      string
	ToolCalls []ToolCall
	Usage     Usage
	// Done is true when the model signalled it has no further tool calls.
	Done bool
}

// Provider is the model-agnostic completion interface.
type Provider interface {
	Name() string
	Model() string
	Complete(ctx context.Context, req Request) (Response, error)
}
