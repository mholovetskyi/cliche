package agent

// Event is a single observable step in the agent loop, used to stream live
// activity to the user (the "cooking" feed) without coupling the loop to any
// particular renderer.
type Event struct {
	Kind   string // "text" | "tool_call" | "tool_result" | "halt" | "budget"
	Turn   int
	Text   string // assistant text (Kind == "text")
	Tool   string // tool name (tool_call / tool_result)
	Detail string // compact human detail
	OK     bool   // tool success (tool_result)
}

// Observer receives Events as they happen. Optional; nil disables streaming.
type Observer func(Event)
