package models

type CanonicalEvent struct {
	Kind      string
	SessionID string
	Message   string

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

type ToolEvent struct {
	ID   string
	Name string

	Target string
	Input  []byte
	Result []byte
	Status string

	Error      string
	DurationMS int
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
	Risk     RiskTier

	Title string

	Question string

	Mode string

	Multi bool

	Options []ChoiceOption

	Questions []PromptQuestion

	Schema []byte
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
