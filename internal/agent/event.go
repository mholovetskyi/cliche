package agent

// Event is a single observable step in the agent loop, used to stream live
// activity to the user (the "cooking" feed) without coupling the loop to any
// particular renderer.
type Event struct {
	Kind   string // "text" | "tool_call" | "tool_result" | "halt" | "budget"
	Turn   int
	Text   string     // assistant text (Kind == "text")
	Tool   string     // tool name (tool_call / tool_result)
	Detail string     // compact human detail
	OK     bool       // tool success (tool_result)
	IsEdit bool       // the tool mutated a file (write/edit) — drives live preview reload
	Path   string     // the file an edit touched (project-relative), for preview scoping
	Images []string   // data: URLs a tool produced (e.g. screenshots), for the UI feed
	Plan   []PlanStep // the agent's live checklist (Kind == "plan")
}

// PlanStep is one item in the agent's self-maintained progress checklist.
type PlanStep struct {
	Title  string `json:"title"`
	Status string `json:"status"` // "pending" | "doing" | "done"
}

// Observer receives Events as they happen. Optional; nil disables streaming.
type Observer func(Event)
