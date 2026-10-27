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

	// Step is one step of the plan, changed alone (plan_step).
	Step *PlanStepEvent

	Raw map[string]any
}

type MessageDelta struct {
	TurnID    string
	MessageID string

	// Index is the increment's position, meaningful only when Sequenced is
	// true: with no index: field mapped every increment reads zero, so the
	// chat shows such a stream live in arrival order and records no chunk.
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

	// SpawnsScope names the subagent this call spawned, AddressesScope one it
	// waits on or closes; a subagent's events are recorded in its scope.
	SpawnsScope    string
	AddressesScope string
}

type ToolLocation struct {
	Path string
	Line int
}

// SubagentEvent is what a subagent's own event says of it. Status is
// Crowbar's word, by the event's status_map.
type SubagentEvent struct {
	ID, AgentType, Title, Status, Result string
	Usage                                *SubagentUsage
}

// SubagentUsage is what a subagent spent; nil fields were not reported.
type SubagentUsage struct {
	Tokens, ToolUses, DurationMs int64
}

// PlanStepEvent is one step of a plan changed alone, in the scope that keeps it.
type PlanStepEvent struct {
	ScopeID, StepID, Text, Detail, Status string
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
	// PlanStepRemoved is a step that leaves the plan.
	PlanStepRemoved = "removed"
)
