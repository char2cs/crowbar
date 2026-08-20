package models

// CanonicalEvent is one provider hook, mapped into Crowbar's vocabulary. Every
// field is optional: a descriptor maps what its provider reports, and what it
// does not map stays zero. That is the whole degradation story — an older CLI, or
// a provider without the concept, lands on the same zero value as a descriptor
// that never declared the field.
//
// It deliberately carries NO provider `source` label. Claude reports
// source=clear where Codex reports source=startup for the very same event, so
// any branch on that vocabulary is provider-specific and will break on the next
// CLI. Carrying it would invite exactly that branch.
type CanonicalEvent struct {
	Kind      string
	SessionID string
	Message   string

	// AsyncWork is how many units of asynchronous work the CLI reports STILL
	// OUTSTANDING as of this event — a LEVEL it re-states every time, not a delta.
	//
	// Provider-agnostic by construction: the descriptor names the array whose
	// LENGTH is that level, and a provider that names nothing reports 0 forever.
	// The engine counts; it never learns what the entries mean.
	//
	// A level, because the edges are a lie. Measured against claude 2.1.212, the
	// SubagentStart/SubagentStop pair does NOT balance — the two hooks observe
	// different populations — so one run gave 4 starts against 9 stops and another
	// 3 starts against 0 stops. Counting edges drifts in BOTH directions: too many
	// stops clear the spinner early, too few strand it ON forever. A re-stated
	// level cannot drift, because every report overwrites the last wholesale.
	AsyncWork int

	// Model is the model the CLI actually resolved. It is the only reliable way to
	// learn that an org allowlist SUBSTITUTED the requested model: the substitute
	// is reported here, before any tokens are spent.
	Model string
	// Effort is the reasoning effort the CLI actually used, when it reports one.
	Effort string
	// Reason accompanies a session ending.
	Reason string

	Tool      *ToolEvent
	Subagent  *SubagentEvent
	Interrupt *InterruptEvent

	// Delta is a chunk of what the agent is saying, on message_delta and nowhere
	// else. See MessageDelta.
	Delta *MessageDelta

	// Failure is why a turn ended badly, on turn_failed and nowhere else.
	Failure *TurnFailure

	// Choice is a prompt the agent is BLOCKED on until a human answers it. It sits
	// beside Interrupt rather than inside it because the two answer different
	// questions: an interruption says the agent stopped, a choice says what it is
	// waiting to be told. A provider whose descriptor maps no choice vocabulary
	// reports the interruption alone, exactly as before.
	Choice *ChoicePrompt

	Raw map[string]any
}

// MessageDelta is one increment of an assistant message, delivered while the
// agent is producing it.
//
// It is the answer to a defect rather than a feature: turn_stop carries the LAST
// message of a turn and no other, so an agent that spoke, used a tool, and spoke
// again reported half of what it said. These carry all of it.
type MessageDelta struct {
	// TurnID and MessageID are the provider's own identities. One turn holds many
	// messages; one message holds many deltas. Both are opaque — Crowbar groups by
	// them and never parses them.
	TurnID    string
	MessageID string

	// Index is this delta's position WITHIN its message, contiguous from zero.
	//
	// It is the integrity check, and the only one there is. Hook delivery has no
	// acknowledgement, so a chunk that never arrives is otherwise a silent hole in
	// what the agent said; a gap in this counter is how a reader knows a message
	// is incomplete rather than short.
	Index int

	// Final marks the last delta of its message.
	//
	// It is NOT guaranteed to arrive. A message the human interrupts simply stops:
	// measured against claude 2.1.236, ESC mid-generation produces no Final, no
	// turn_stop, and no other hook of any kind. An unfinished message is therefore
	// evidence in its own right, and the usecase reads it as such.
	Final bool

	// Text is the increment itself — never a cumulative snapshot of the message so
	// far. Concatenating a message's deltas in Index order reproduces the
	// provider's own report of that message exactly.
	Text string
}

// TurnFailure is a turn the provider ended because it could not run it.
//
// Distinct from an interruption, which is a human deciding to stop, and from
// turn_stop, which is a turn that finished. A provider fires this INSTEAD OF
// turn_stop, so a chat that maps only the latter is left with a turn nothing ever
// closes.
type TurnFailure struct {
	// Reason is the provider's own classification, carried verbatim and never
	// interpreted. claude 2.1.236 reports one of eleven: authentication_failed,
	// oauth_org_not_allowed, account_on_hold, billing_error, rate_limit,
	// overloaded, invalid_request, model_not_found, server_error, unknown,
	// max_output_tokens.
	Reason string
	// Detail is the provider's longer explanation when it sends one.
	Detail string
}

// ToolEvent is one tool invocation or completion. Input and Result are the raw
// payload subtrees the descriptor pointed at, carried as bytes because they go to
// the content store and must never be inlined into an event log.
type ToolEvent struct {
	ID   string
	Name string
	// Target is the one field worth indexing across tools — the file path, the
	// command, the URL. A descriptor picks it per provider; an unmapped target is
	// empty, which is legible, where a guessed one would be wrong.
	Target string
	Input  []byte
	Result []byte
	Status string
	// Error is what the provider said went wrong, when it reports a failure as a
	// distinct event rather than as a status. It is a MESSAGE, not a payload: the
	// full failure text travels through Result to the content store.
	Error      string
	DurationMS int
}

type SubagentEvent struct {
	ID        string
	AgentType string
}

// Interruption kinds. These are Crowbar concepts: a provider's own event names
// map onto them, and a provider missing one simply never reports it.
const (
	InterruptPermission   = "permission"
	InterruptNotification = "notification"
	InterruptElicitation  = "elicitation"
	InterruptCompaction   = "compaction"
)

// InterruptEvent is the agent being blocked on, or interrupted by, something
// outside the turn — a permission prompt, a notification, a compaction.
//
// These are the events whose absence caused every legibility failure observed in
// live testing: a provider blocked on a trust prompt showed nothing, and a
// compaction was invisible.
type InterruptEvent struct {
	Kind     string
	Detail   string
	Resolved bool
}

// Choice kinds. What KIND of answer the agent is waiting for, in Crowbar's own
// vocabulary — a provider's own event or tool names map onto these.
const (
	// ChoiceToolPermission is "may I run this tool".
	ChoiceToolPermission = "tool_permission"
	// ChoiceQuestion is the agent asking the human to pick between labelled
	// options. On claude this arrives as the AskUserQuestion TOOL rather than as an
	// event of its own (measured against claude 2.1.234 on 2026-08-17), which is
	// why it is recognised by the shape of a permission's tool input and not by a
	// hook name.
	ChoiceQuestion = "question"
	// ChoiceElicitation is an MCP server asking through the CLI.
	ChoiceElicitation = "elicitation"
)

// Choice option kinds. A client renders by kind rather than by label, because the
// labels of the synthetic ones are Crowbar's words and not a provider's.
const (
	// ChoiceOptionAnswer is one of the labelled answers the agent itself offered.
	ChoiceOptionAnswer = "answer"
	// ChoiceOptionAllow and ChoiceOptionDeny are the two answers every permission
	// prompt has by construction. No provider enumerates them, so Crowbar does.
	ChoiceOptionAllow = "allow"
	ChoiceOptionDeny  = "deny"
	// ChoiceOptionSuggestion is a broader grant the provider proposed alongside the
	// plain allow — claude's permission_suggestions, e.g. "and stop asking about
	// this directory".
	ChoiceOptionSuggestion = "suggestion"
)

// ChoicePrompt is the agent waiting on a human decision.
//
// Every field is optional in the same way the rest of this file's are: a
// descriptor maps what its provider reports, and an unmapped field stays zero. A
// descriptor that maps none of the choice vocabulary produces no ChoicePrompt at
// all, which is how this degrades to absent rather than to a prompt with nothing
// in it.
type ChoicePrompt struct {
	Kind string
	// PromptID is the provider's OWN id for this prompt, when it gives one. It is
	// not Crowbar's identity for the record — see domain.ActivityChoice.ID — but it
	// is what lets two hooks describing the same prompt be recognised as one.
	PromptID string
	// ToolName is the tool a permission is gating. It is the ONLY correlation a
	// permission offers on claude: measured against 2.1.234 on 2026-08-17, a
	// PermissionRequest carries NO tool_use_id despite claude's own docs claiming
	// one, so the in-flight PreToolUse of the same name is what the prompt attaches
	// to.
	ToolName string
	// Title is a short header — the tool name for a permission, the question's own
	// header, the MCP server's name for an elicitation.
	Title string
	// Question is the text actually being asked.
	Question string
	// Mode is the provider's own word for how it expects to be answered (claude's
	// elicitation `mode`), carried verbatim and never interpreted here.
	Mode string
	// Multi reports that more than one option may be chosen. It describes the
	// PROMPT-level Options below; a question carries its own, because one payload
	// can mix a single-select question with a multi-select one.
	Multi bool
	// Options are the answers the PROMPT itself offers — a permission's allow, deny
	// and suggestions, and nothing else. A question-kind prompt leaves this empty
	// and puts every option inside Questions, so there is exactly one place an
	// answerable question's options can be.
	Options []ChoiceOption
	// Questions is the whole of what a question-kind prompt is asking.
	//
	// It is a LIST because claude's AskUserQuestion input is one, and a payload
	// carrying three questions is one prompt with three of them rather than three
	// prompts. Modelling only the first is what stranded an agent: the answer it
	// produced covered one question, the CLI was handed a partial `updatedInput`,
	// and it went on asking for answers 2 and 3 that no surface could ever send.
	//
	// A one-question payload yields a list of length ONE, so nothing anywhere
	// branches on how many there are.
	Questions []PromptQuestion
	// Schema is the provider's requested-input schema, verbatim, for a prompt whose
	// answer is a FORM rather than a choice between options.
	Schema []byte
}

// PromptQuestion is ONE question inside a prompt, with the options that answer it.
//
// It is not called ChoiceQuestion because that name is already taken by the KIND
// of prompt this appears on — models.ChoiceQuestion is the string "question".
//
// Multi is per question and not per prompt, because the provider says so: claude
// carries multiSelect on each entry of the questions array, so one prompt can ask
// "pick one" and "pick any" in the same breath. A prompt-level flag would have to
// pick one of the two answers and be wrong about the other.
type PromptQuestion struct {
	// ID is stable within its prompt. It is what groups a flat list of picked
	// option ids back into per-question answers.
	ID    string
	Title string
	// Text is the question as asked. It is also the KEY the answer is filed under
	// on the way back to claude, which is why it is carried rather than derived.
	Text    string
	Multi   bool
	Options []ChoiceOption
}

// ChoiceOption is one answer a prompt will accept.
type ChoiceOption struct {
	// ID is stable within its PROMPT — not merely within its question — because an
	// answer names its picks in one flat list and nothing in that list says which
	// question a pick belongs to. Two questions offering an option each called
	// "answer-0" would make that list ambiguous.
	ID          string
	Kind        string
	Label       string
	Description string
}
