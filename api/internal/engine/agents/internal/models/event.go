package models

type CanonicalEvent struct {
	Kind      string
	SessionID string
	Message   string

	TurnID string

	TurnStatus string

	Model string

	Effort string

	Reason string

	Tool      *ToolEvent
	Subagent  *SubagentEvent
	Interrupt *InterruptEvent

	Delta *MessageDelta

	Complete *MessageComplete

	Failure *TurnFailure

	Choice *ChoicePrompt

	Plan []PlanStep

	Step *PlanStepEvent

	Shells []ShellEvent

	Raw map[string]any
}

type MessageDelta struct {
	TurnID    string
	MessageID string

	Index     int
	Sequenced bool

	Final bool

	Text string
}

type MessageComplete struct {
	TurnID    string
	MessageID string
	Text      string
	Phase     string
}

type TurnFailure struct {
	Reason string

	Kind string

	Detail string
}

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

	Title       string
	MCPServer   string
	MCPTool     string
	ExitCode    *int
	Interrupted bool

	EditBefore string
	EditAfter  string
	Output     string
	Cwd        string
	Diff       string
	Patch      []PatchHunk

	Error      string
	DurationMS int

	SpawnsScope    string
	AddressesScope string

	SpawnsShell     string
	StopsShell      string
	ShellKind       string
	ShellPersistent bool
	ShellTimeoutMS  int
	ShellRef        string
}

type ToolLocation struct {
	Path string
	Line int
}

type SubagentEvent struct {
	ID, AgentType, Title, Status, Result string
	Usage                                *SubagentUsage
}

type SubagentUsage struct {
	Tokens, ToolUses, DurationMs int64
}

type ShellEvent struct {
	ID, Status, Summary, Note string
	ExitCode                  *int
}

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
	Reason      bool
}

type PlanStep struct {
	Text   string
	Status string
}

const (
	PlanStepPending = "pending"
	PlanStepActive  = "active"
	PlanStepDone    = "done"
	PlanStepRemoved = "removed"
)
