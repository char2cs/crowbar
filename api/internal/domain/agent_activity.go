package domain

import "time"

// Turn roles.
const (
	TurnRoleUser      = "user"
	TurnRoleAssistant = "assistant"
)

// Tool-call statuses.
const (
	ToolStatusRunning   = "running"
	ToolStatusOK        = "ok"
	ToolStatusError     = "error"
	ToolStatusAbandoned = "abandoned"
)

// MaxOpenPerTurn bounds how many in-flight items one turn's state may hold.
//
// A provider that opens a tool call and never reports its completion would
// otherwise grow the aggregate without limit, and the whole point of this
// aggregate is that its size is independent of how long a chat runs. The cap is
// far above any real turn's concurrency, so reaching it means a provider stopped
// reporting, not that a user did something unusual.
const MaxOpenPerTurn = 64

// AgentActivity is a chat's conversation record — but only the part of it that is
// still OPEN, plus the single item the current event just touched.
//
// This shape is forced by how the event store works. Events are RFC-6902 patches
// over state, snapshots are the whole state written as one row, and a cold load
// materialises all of it. An aggregate holding every turn would therefore make
// both costs grow with conversation length, on exactly the longest and most
// valuable chats. Holding only open state keeps them flat forever.
//
// What that costs is that this type cannot answer "what did this chat say" — only
// the projection can. That is deliberate. The projection is durable, is rebuilt by
// replaying the log when lost, and is what every reader was always going to query.
//
// Last is the channel through which completed work reaches that projection. A
// command contributes exactly two things to an event — its name and the patch —
// so state is the ONLY way a projection can observe anything. Each command
// overwrites Last with the one item it touched, and the item's data lives forever
// in the patch that wrote it.
type AgentActivity struct {
	ChatID string `json:"chatId"`

	// Seq orders items within a chat. It is the aggregate's own counter rather
	// than a timestamp because two hooks can land in the same millisecond and a
	// conversation that renders out of order is worse than one that renders late.
	Seq int64 `json:"seq"`

	// Turn is the open assistant turn, if one is running.
	Turn *ActivityTurn `json:"turn,omitempty"`

	// In-flight items belonging to the open turn, keyed by provider-supplied id.
	Tools         map[string]ActivityToolCall     `json:"tools,omitempty"`
	Subagents     map[string]ActivitySubagent     `json:"subagents,omitempty"`
	Interruptions map[string]ActivityInterruption `json:"interruptions,omitempty"`

	// Choices are the prompts this chat is still waiting on a human to answer.
	// They are open state in the strictest sense the aggregate has: one opens when
	// the provider asks and closes the moment anything shows the question is no
	// longer being asked.
	Choices map[string]ActivityChoice `json:"choices,omitempty"`

	Last *ActivityDelta `json:"last,omitempty"`
}

// Delta phases and kinds.
const (
	DeltaOpen  = "open"
	DeltaClose = "close"

	DeltaTurn         = "turn"
	DeltaTool         = "tool"
	DeltaSubagent     = "subagent"
	DeltaInterruption = "interruption"
	DeltaChoice       = "choice"
)

// ActivityDelta is what one command touched. Exactly one of the pointers is set,
// chosen by Kind.
type ActivityDelta struct {
	Phase string `json:"phase"`
	Kind  string `json:"kind"`

	// SupersededTurnID is the id the OPEN turn carried, when a close lands under a
	// different one.
	//
	// It exists because the two ids are minted for different reasons. The open turn
	// is minted when a prompt arrives, so tool calls have something to attach to
	// before any reply exists; the closed turn is keyed on the hook's DELIVERY id,
	// so a redelivered reply rewrites one row instead of appending a second copy.
	// The projection re-points the activity from the first to the second, and the
	// UI can then say which tool calls produced which answer.
	SupersededTurnID string `json:"supersededTurnId,omitempty"`

	Turn         *ActivityTurn         `json:"turn,omitempty"`
	Tool         *ActivityToolCall     `json:"tool,omitempty"`
	Subagent     *ActivitySubagent     `json:"subagent,omitempty"`
	Interruption *ActivityInterruption `json:"interruption,omitempty"`
	Choice       *ActivityChoice       `json:"choice,omitempty"`
}

// ActivityTurn is one side of the conversation.
type ActivityTurn struct {
	ID         string `json:"id"`
	ChatID     string `json:"chatId"`
	Seq        int64  `json:"seq"`
	Role       string `json:"role"`
	ProviderID string `json:"providerId"`
	RunnerID   string `json:"runnerId"`
	SessionID  string `json:"sessionId"`

	// Text is hook-confirmed: a successful API call never manufactures it.
	Text   string `json:"text"`
	Effort string `json:"effort,omitempty"`

	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

// ActivityToolCall is one tool invocation. The payloads are NOT here: RequestRef
// and ResultRef address them in the content store, so an event never carries a
// tool's output and a snapshot never rewrites it.
type ActivityToolCall struct {
	ID     string `json:"id"`
	TurnID string `json:"turnId"`
	ChatID string `json:"chatId"`
	Seq    int64  `json:"seq"`

	Name string `json:"name"`
	// Target is the one field worth indexing across tools — the file, the
	// command, the URL. Unmapped means empty, which is legible; a guessed one
	// would be wrong.
	Target string `json:"target,omitempty"`

	RequestRef string `json:"requestRef,omitempty"`
	ResultRef  string `json:"resultRef,omitempty"`

	Status string `json:"status"`
	// Error is a SHORT copy of why a failing tool failed, so a timeline can say it
	// without fetching a payload. The full text is at ResultRef.
	Error      string `json:"error,omitempty"`
	DurationMS int    `json:"durationMs,omitempty"`

	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

type ActivitySubagent struct {
	ID        string     `json:"id"`
	TurnID    string     `json:"turnId"`
	ChatID    string     `json:"chatId"`
	Seq       int64      `json:"seq"`
	AgentType string     `json:"agentType,omitempty"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

// ActivityInterruption is the agent being blocked on, or interrupted by,
// something outside the turn. These are the events whose absence made a working
// agent look like a frozen one.
type ActivityInterruption struct {
	ID         string     `json:"id"`
	TurnID     string     `json:"turnId"`
	ChatID     string     `json:"chatId"`
	Seq        int64      `json:"seq"`
	Kind       string     `json:"kind"`
	Detail     string     `json:"detail,omitempty"`
	At         time.Time  `json:"at"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
}

// OpenCount is how many in-flight items the aggregate currently holds. It is what
// MaxOpenPerTurn is checked against, and what a test asserts to prove state stays
// bounded over a long conversation.
func (a AgentActivity) OpenCount() int {
	return len(a.Tools) + len(a.Subagents) + len(a.Interruptions) + len(a.Choices)
}

// Choice kinds — what kind of answer the agent is blocked waiting for.
const (
	ChoiceKindPermission  = "tool_permission"
	ChoiceKindQuestion    = "question"
	ChoiceKindElicitation = "elicitation"
)

// Choice option kinds. A client renders by kind rather than by label: the labels
// of allow and deny are Crowbar's own words, because no provider enumerates the
// two answers every permission prompt has by construction.
const (
	ChoiceOptionAnswer     = "answer"
	ChoiceOptionAllow      = "allow"
	ChoiceOptionDeny       = "deny"
	ChoiceOptionSuggestion = "suggestion"
)

// How a pending choice stopped being pending.
const (
	// ChoiceResolutionAnswered is a decision that came back THROUGH Crowbar. It is
	// the value the answer channel will write; nothing writes it today, and that is
	// deliberate — this iteration observes prompts, it does not answer them.
	ChoiceResolutionAnswered = "answered"
	// ChoiceResolutionProceeded is the work being observed to move on: the gated
	// tool completed, so the question was answered — at the PTY, by a human typing
	// into the vendor CLI, entirely outside Crowbar. That path is the COMMON one and
	// it fires no resolution event of its own, which is why the completion of the
	// gated work has to count as one. A prompt that never clears strands the UI on a
	// question nobody is asking any more, which is worse than clearing early.
	ChoiceResolutionProceeded = "proceeded"
	// ChoiceResolutionAbandoned is the turn ending with the prompt still open.
	ChoiceResolutionAbandoned = "abandoned"
)

// ActivityChoice is the agent waiting on a HUMAN DECISION — a tool permission, a
// question it asked, an MCP server's elicitation.
//
// It is the one piece of open state that a user can act on, so it carries enough
// to be rendered as a real prompt rather than as a spinner: what kind of answer is
// wanted, what is being asked, and which answers exist.
//
// ID is Crowbar's own stable identity for the prompt, derived from the provider's
// PromptID where there is one and minted otherwise. It is stable across the open
// and every later write, and every Option carries a stable id of its own within
// it — which together are what an answer will name when the answer channel lands.
// Adding that channel is additive: a command that sets Resolution to
// ChoiceResolutionAnswered and records which option ids were chosen changes
// nothing here.
type ActivityChoice struct {
	ID     string `json:"id"`
	TurnID string `json:"turnId"`
	ChatID string `json:"chatId"`
	Seq    int64  `json:"seq"`

	Kind string `json:"kind"`
	// PromptID is the PROVIDER's own id for this prompt, when it gives one. It is
	// not the identity above; it is what lets two payloads describing the same
	// prompt be recognised as one.
	PromptID string `json:"promptId,omitempty"`
	// ToolID and ToolName are the gated tool call. ToolID is adopted from the
	// in-flight invocation rather than read from the payload: a claude permission
	// carries no tool_use_id at all (measured against 2.1.234 on 2026-08-17).
	ToolID   string `json:"toolId,omitempty"`
	ToolName string `json:"toolName,omitempty"`

	Title    string                 `json:"title,omitempty"`
	Question string                 `json:"question,omitempty"`
	Mode     string                 `json:"mode,omitempty"`
	Multi    bool                   `json:"multi,omitempty"`
	Options  []ActivityChoiceOption `json:"options,omitempty"`
	// Schema is the provider's requested-input schema, verbatim, for a prompt whose
	// answer is a form rather than a pick from Options.
	Schema string `json:"schema,omitempty"`

	At         time.Time  `json:"at"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
	Resolution string     `json:"resolution,omitempty"`
}

// ActivityChoiceOption is one answer a prompt will accept.
type ActivityChoiceOption struct {
	// ID is stable within its prompt, so an answer names an option rather than
	// echoing its label back.
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

// Pending reports whether this prompt is still waiting on a human.
func (c ActivityChoice) Pending() bool { return c.ResolvedAt == nil }
