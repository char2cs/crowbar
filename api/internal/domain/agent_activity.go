package domain

import (
	"fmt"
	"time"
)

// Turn roles.
//
// A turn's role names WHO PUT THE TEXT THERE, and the set is four because four
// distinct authors can. The two obvious ones are the human and the CLI's model.
// The other two exist because a chat receives text neither of them wrote, and
// recording it under one of their names is a lie the rest of the system then acts
// on — a sibling agent reading the chat log takes a harness notification for
// something the human said, and a user reads a daemon's observation as the model's
// answer.
const (
	// TurnRoleUser is the human. Everything Crowbar's own composer sends, and
	// anything typed straight into the CLI's PTY, is theirs.
	TurnRoleUser = "user"

	// TurnRoleAssistant is the provider's model.
	TurnRoleAssistant = "assistant"

	// TurnRoleHarness is text the vendor CLI's own harness INJECTED into the
	// conversation as if it were a prompt — a background-subagent completion
	// notification is the measured case. The agent genuinely received it and its
	// next answer refers to it, so dropping it would leave a reply with no
	// antecedent; attributing it to the user would put words in their mouth.
	TurnRoleHarness = "harness"

	// TurnRoleNotice is Crowbar's own observation about the chat, carrying a
	// provider's verbatim words — the usage-limit banner a CLI painted on its
	// screen instead of ending its turn. It is never model output and never a
	// prompt anybody sent.
	TurnRoleNotice = "notice"
)

// KnownTurnRole reports whether role is one the ledger accepts. It is the single
// membership test, so a new role is added in exactly one place.
func KnownTurnRole(role string) bool {
	switch role {
	case TurnRoleUser, TurnRoleAssistant, TurnRoleHarness, TurnRoleNotice:
		return true
	default:
		return false
	}
}

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
	// Questions is the whole of what a question-kind prompt is asking, one entry
	// per question. A prompt that asks three things is ONE record with three
	// questions, because the provider gates it as one call and expects one answer
	// covering all of them.
	//
	// It is empty for a permission and an elicitation, which offer Options and a
	// Schema instead — and it is empty for a question RECORDED BEFORE this field
	// existed, which is a graceful fallback rather than a migration: such a row
	// still has its prompt-level Question and Options and still answers exactly as
	// it did.
	Questions []ActivityChoiceQuestion `json:"questions,omitempty"`
	// Schema is the provider's requested-input schema, verbatim, for a prompt whose
	// answer is a form rather than a pick from Options.
	Schema string `json:"schema,omitempty"`

	At         time.Time  `json:"at"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
	Resolution string     `json:"resolution,omitempty"`
}

// ActivityChoiceQuestion is one question inside a prompt, with the options that
// answer it.
//
// Multi is per question because the provider says so — claude carries multiSelect
// on each entry of its questions array — so one prompt can ask "pick one" and
// "pick any" at the same time.
type ActivityChoiceQuestion struct {
	// ID is stable within its prompt, and is what groups a flat list of picked
	// option ids back into per-question answers.
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
	// Text is the question as asked, and also the KEY the answer is filed under on
	// the way back to the provider.
	Text    string                 `json:"text,omitempty"`
	Multi   bool                   `json:"multi,omitempty"`
	Options []ActivityChoiceOption `json:"options,omitempty"`
}

// ActivityChoiceOption is one answer a prompt will accept.
type ActivityChoiceOption struct {
	// ID is stable within its PROMPT — not merely within the question that offered
	// it — because an answer names its picks in one flat list and nothing in that
	// list says which question a pick belongs to.
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

// Pending reports whether this prompt is still waiting on a human.
func (c ActivityChoice) Pending() bool { return c.ResolvedAt == nil }

// ChoiceAnswer is one question and the options a human picked for it.
type ChoiceAnswer struct {
	Question ActivityChoiceQuestion
	Picked   []ActivityChoiceOption
}

// AskedQuestions is what this prompt is asking, in ONE shape whatever wrote it.
//
// A prompt recorded since questions were modelled carries them directly. A
// permission, and a question recorded before that field existed, carries a
// prompt-level question text with a flat option list instead — which is the same
// thing with one entry, so it is presented as one entry. Callers therefore never
// branch on which of the two a record happens to be.
//
// An elicitation has neither: its answer is a form and its "options" are the MCP
// verbs the client invents, so it yields nothing here and its picks are checked
// against nothing.
func (c ActivityChoice) AskedQuestions() []ActivityChoiceQuestion {
	if len(c.Questions) > 0 {
		return c.Questions
	}
	if len(c.Options) == 0 {
		return nil
	}
	question := ActivityChoiceQuestion{ID: "q0", Multi: c.Multi, Options: c.Options}
	if c.Kind == ChoiceKindQuestion {
		// Only a QUESTION's title and text are a question's. A permission's Title is
		// the gated TOOL's name, and borrowing it here would put "Bash" in a refusal
		// as though the tool were the thing being asked — a message that reaches a
		// person through the 400.
		question.Title, question.Text = c.Title, c.Question
	}
	return []ActivityChoiceQuestion{question}
}

// ResolvePicks turns the FLAT list of option ids an answer names into one answer
// per question, refusing anything that is not a complete answer to this prompt.
//
// It lives on the domain type because two layers have to enforce the identical
// rule — the usecase, which turns picks into a provider payload, and the
// aggregate, which records the decision — and they used to enforce different
// strictnesses. A rule written twice is a rule that will disagree with itself, and
// the disagreement here would be a 400 the UI could not predict or an answer the
// aggregate accepted and the CLI could not use.
//
// The rules, and what each of them is stopping:
//
//   - Every question must be answered. A partial answer is what stranded a live
//     agent: claude was handed picks for one of three questions, said "still
//     waiting on your answers to questions 2 & 3", and no surface could ever send
//     them. Partial must therefore be impossible rather than discouraged.
//   - A question that is not multi-select takes ONE pick. Two picks on it are not
//     an answer the provider has any way to read.
//   - Every pick must name an option this prompt actually offered, and may name it
//     only once.
//
// A prompt with nothing to pick from (an elicitation) returns no answers and no
// error: its ids are the provider's own verbs, and the descriptor's response
// templates are what decide whether a verb means anything.
func (c ActivityChoice) ResolvePicks(optionIDs []string) ([]ChoiceAnswer, error) {
	questions := c.AskedQuestions()
	if len(questions) == 0 {
		return nil, nil
	}

	answers := make([]ChoiceAnswer, len(questions))
	for i, question := range questions {
		answers[i] = ChoiceAnswer{Question: question}
	}
	seen := make(map[string]bool, len(optionIDs))
	for _, id := range optionIDs {
		at, option, offered := findOption(questions, id)
		if !offered {
			return nil, fmt.Errorf("%q is not an option on this prompt", id)
		}
		if seen[id] {
			return nil, fmt.Errorf("%q was picked more than once", id)
		}
		seen[id] = true
		answers[at].Picked = append(answers[at].Picked, option)
	}

	for _, answer := range answers {
		switch {
		case len(answer.Picked) == 0:
			return nil, fmt.Errorf("%s has no answer, and every question must be answered",
				questionName(answer.Question))
		case len(answer.Picked) > 1 && !answer.Question.Multi:
			return nil, fmt.Errorf("%s takes one answer, got %d",
				questionName(answer.Question), len(answer.Picked))
		}
	}
	return answers, nil
}

// findOption locates an option id across every question, since an answer names
// its picks without saying which question each belongs to.
func findOption(
	questions []ActivityChoiceQuestion,
	id string,
) (int, ActivityChoiceOption, bool) {
	for i, question := range questions {
		for _, option := range question.Options {
			if option.ID == id {
				return i, option, true
			}
		}
	}
	return 0, ActivityChoiceOption{}, false
}

// questionName is how a question is referred to in a refusal, ALREADY QUOTED
// where there is something to quote.
//
// The text is what the human was shown, so it is what makes the message legible —
// and with several questions on one prompt it is the only thing that says WHICH
// of them was left unanswered or double-picked. A prompt with nothing to name is
// referred to as itself rather than by an internal id nobody has seen.
func questionName(q ActivityChoiceQuestion) string {
	switch {
	case q.Text != "":
		return fmt.Sprintf("%q", q.Text)
	case q.Title != "":
		return fmt.Sprintf("%q", q.Title)
	default:
		return "this prompt"
	}
}

// AnswerKey is the TEXT a question's answer is filed under on the way back to the
// provider — claude's `answers` object is keyed by the question string it sent.
//
// Empty means this question cannot be keyed at all, which is only reachable for a
// provider that sent neither text nor header for it.
func (q ActivityChoiceQuestion) AnswerKey() string {
	if q.Text != "" {
		return q.Text
	}
	return q.Title
}
