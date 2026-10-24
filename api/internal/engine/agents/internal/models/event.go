package models

type CanonicalEvent struct {
	Kind      string
	SessionID string
	Message   string

	// TurnID is the provider's own turn identifier, when its descriptor maps
	// one: a turn end can belong to a compaction's own turn, and only the id
	// tells it apart from a reply.
	TurnID string

	// TurnStatus is the provider-reported terminal state when its completion
	// event distinguishes completed from interrupted. Descriptors normalize it.
	TurnStatus string

	AsyncWork int

	Model string

	Effort string

	Reason string

	Tool      *ToolEvent
	Subagent  *SubagentEvent
	Interrupt *InterruptEvent

	Delta *MessageDelta

	// Complete is the whole text of one finished message.
	Complete *MessageComplete

	Failure *TurnFailure

	Choice *ChoicePrompt

	// Plan is the agent's own running to-do list for this turn, newest state
	// wholesale. Nil when the event carries none.
	Plan []PlanStep

	Raw map[string]any
}

type MessageDelta struct {
	TurnID    string
	MessageID string

	// Index is the increment's position, meaningful only when Sequenced is
	// true. A provider whose descriptor maps no index: field (its deltas
	// arrive on one ordered stream with nothing to number) leaves this zero
	// on every increment — reading it then would fold every chunk onto the
	// same slot, so stream.Streams instead assigns arrival order itself.
	Index     int
	Sequenced bool

	Final bool

	Text string
}

// MessageComplete is one finished assistant message, whole.
type MessageComplete struct {
	TurnID    string
	MessageID string
	Text      string
	Phase     string
}

type TurnFailure struct {
	Reason string

	Detail string
}

// PatchHunk is one unified-diff hunk: Lines carry the " ", "-", "+" prefixes.
type PatchHunk struct {
	OldStart int
	NewStart int
	Lines    []string
}

type ToolEvent struct {
	ID        string
	Name      string
	Kind      string
	Locations []ToolLocation

	Target string
	Input  []byte
	Result []byte
	Status string

	// Title is the agent's own one-line description of the call.
	Title string
	// MCPServer and MCPTool name the server and tool of a call into one.
	MCPServer string
	MCPTool   string
	// ExitCode is a command's exit status; nil when the payload carries none, so 0 is an answer.
	ExitCode    *int
	Interrupted bool

	// EditBefore/EditAfter are the text an edit replaced and its replacement,
	// when the descriptor maps them; a new file has an empty EditBefore.
	EditBefore string
	EditAfter  string
	// Diff is a unified preview of this tool call's own changed files.
	Diff string
	// Patch is the provider's own hunks for the change a completed tool made,
	// with true line numbers; it supersedes the EditBefore/EditAfter snippet.
	Patch []PatchHunk

	Error      string
	DurationMS int

	// NestedSessionID names a second conversation this call opened or
	// addresses, riding the same connection; its events are routed to it.
	NestedSessionID string
}

type ToolLocation struct {
	Path string
	Line int
}

type SubagentEvent struct {
	ID        string
	AgentType string
}

const (
	InterruptPermission   = "permission"
	InterruptNotification = "notification"
	InterruptElicitation  = "elicitation"
	InterruptCompaction   = "compaction"
)

type InterruptEvent struct {
	Kind     string
	Detail   string
	Resolved bool
}

const (
	ChoiceToolPermission = "tool_permission"

	ChoiceQuestion = "question"

	ChoiceElicitation = "elicitation"
)

const (
	ChoiceOptionAnswer = "answer"

	ChoiceOptionAllow = "allow"
	ChoiceOptionDeny  = "deny"

	ChoiceOptionSuggestion = "suggestion"
)

type ChoicePrompt struct {
	Kind string

	PromptID string

	ToolName string

	Title string

	Question string

	Mode string

	Multi bool

	Options []ChoiceOption

	Questions []PromptQuestion

	Schema []byte

	// ToolTarget, ToolInput and MCPServer say which call a tool question or
	// permission is about, so it can be matched to that call's row.
	ToolTarget string
	ToolInput  []byte
	MCPServer  string
}

type PromptQuestion struct {
	ID    string
	Title string

	Text    string
	Multi   bool
	Options []ChoiceOption
}

type ChoiceOption struct {
	ID          string
	Kind        string
	Label       string
	Description string
}

// PlanStep is one entry of an agent's running plan.
//
// Status is CROWBAR'S word, not the provider's: a descriptor translates its own
// vocabulary with `steps.status_map` (the same mechanism permission's
// suggestion_label.* uses), so Go never learns any provider's spelling of
// "in progress".
type PlanStep struct {
	Text   string
	Status string
}

// The step statuses Crowbar renders. A descriptor whose status_map produces
// anything else leaves the step unstyled rather than failing — an unknown status
// is still a step worth showing.
const (
	PlanStepPending = "pending"
	PlanStepActive  = "active"
	PlanStepDone    = "done"
)
