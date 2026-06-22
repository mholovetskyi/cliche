// Package provider defines the model-agnostic interface every backend
// implements. Cliche is BYO-key and provider-neutral by design: the Trust
// Kernel wraps whatever model you bring. The mock provider powers tests and
// the offline demo; the anthropic provider is the first real backend.
package provider

import "context"

// ToolCall is a request from the model to run a tool.
type ToolCall struct {
	Name string            `json:"name"`
	Args map[string]string `json:"args"`
	// Signature is a stable description of the call used by the Governor for
	// repetition detection. Two semantically-identical calls must share a
	// signature.
	Signature string `json:"signature"`
}

// Usage reports token consumption for a single completion.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Message is one turn of conversation history.
type Message struct {
	Role    string
	Content string
}

// Request is a single completion request.
type Request struct {
	System   string
	Prompt   string
	Model    string
	Messages []Message
	// MaxOutputTokens, when > 0, bounds this request's output. The agent sets
	// it from the remaining token budget so a single turn cannot overshoot the
	// hard token cap. Backends must honor it (clamping to their own ceiling).
	MaxOutputTokens int
}

// Response is a single completion result.
type Response struct {
	Text      string
	ToolCalls []ToolCall
	Usage     Usage
	// Done is true when the model signals the task is complete.
	Done bool
}

// Provider is the model-agnostic completion interface.
type Provider interface {
	Name() string
	Model() string
	Complete(ctx context.Context, req Request) (Response, error)
}
