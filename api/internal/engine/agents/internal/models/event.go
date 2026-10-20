package models

type CanonicalEvent struct {
	Kind      string
	SessionID string
	Message   string

	// TurnID is the provider's own turn identifier, when its descriptor maps one.
	// A turn-close that also wraps a compaction round trip is told apart from an
	// ordinary reply only by the id compact_pre armed (the turn package's compaction latch).
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

	// NestedSessionID names a second conversation this call's completion refers to
	// (the agent it spawned or addressed); empty otherwise. The turn package routes it.
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
	// InterruptStopped is Crowbar's own doing, not a provider hook: recorded when
	// a person cuts an in-flight turn short, so the transcript can draw the same
	// kind of durable, sequence-anchored divider a compaction gets instead of a
	// client-side guess about what it just clicked.
	InterruptStopped = "stopped"
	// InterruptProviderSwitched, InterruptModelChanged and InterruptEffortChanged
	// are ALSO Crowbar's own doing, same as InterruptStopped: nothing a provider
	// reports, recorded the instant Crowbar itself commits to a provider switch or
	// a model/effort selection change, so the transcript can mark it with the same
	// durable, sequence-anchored divider. Detail carries the new value (the target
	// provider id, model id, or effort level).
	InterruptProviderSwitched = "provider_switched"
	InterruptModelChanged     = "model_changed"
	InterruptEffortChanged    = "effort_changed"
	// InterruptInferred is ALSO Crowbar's own doing, not a provider hook — but
	// unlike the three above, it is not something Crowbar decided: it is
	// Crowbar's own INFERENCE that a turn was interrupted, drawn from silence
	// rather than observed directly. Recorded when a fuse elsewhere in the
	// hook ingress (the message-quiet timeout an assistant reply stopped
	// growing under, with nothing else open to explain why) decides a turn
	// is never going to close on its own — the only trace a CLI's own
	// hookless abort (an ESC/Ctrl+C it reports to nobody) leaves behind.
	InterruptInferred = "inferred"
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
