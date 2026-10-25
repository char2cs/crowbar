package dto

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// AgentMessageDTO is one complete hook-derived message in a chat. Sequence is
// Crowbar's per-chat ledger append order; provider-owned transcript identifiers
// and paths never cross this boundary.
type AgentMessageDTO struct {
	Sequence int `json:"sequence"`
	// DisplayOrder/ItemIndex are what a client actually sorts by — reserved
	// at dispatch time, unlike Sequence, which is the row's persist-order
	// identity and paging cursor. See domain.ActivityTurn.
	DisplayOrder int64 `json:"displayOrder"`
	ItemIndex    int   `json:"itemIndex"`
	// TurnID is what the activity record attaches tool calls to, so a client can
	// show which tools produced which reply. A user turn Crowbar dispatched is
	// named by that prompt's clientRequestId.
	TurnID     string `json:"turnId"`
	Role       string `json:"role"`
	ProviderID string `json:"providerId"`
	Text       string `json:"text"`
	// Effort is the reasoning effort the provider itself reported for this turn.
	// It is NOT the same fact as the chat's requested selection: it is what the CLI
	// says it actually used. Absent when the provider reported none, and never
	// inferred from the request.
	Effort string    `json:"effort,omitempty"`
	At     time.Time `json:"at"`
}

// AgentMessagePageDTO is a bounded chronological ledger window. Cursor is the
// newest item in this page; OldestCursor is the oldest and is used to request
// the page above it. HasMore is directional (newer for after, older otherwise).
type AgentMessagePageDTO struct {
	Cursor       int               `json:"cursor"`
	OldestCursor int               `json:"oldestCursor"`
	HasMore      bool              `json:"hasMore"`
	Items        []AgentMessageDTO `json:"items"`
}

// AgentActivityDTO is what the agent DID during a chat, as distinct from what it
// said. Every list is independently present: an agent that reports no tool
// activity yields an empty one, and the client renders nothing rather than a
// disabled control implying breakage.
type AgentActivityDTO struct {
	ToolCalls     []AgentToolCallDTO          `json:"toolCalls"`
	Subagents     []AgentSubagentDTO          `json:"subagents"`
	Interruptions []AgentInterruptionDTO      `json:"interruptions"`
	Choices       []AgentChoiceDTO            `json:"choices"`
	Components    []AgentActivityComponentDTO `json:"components"`
}

// AgentActivityComponentDTO is the provider-neutral ordered activity stream.
// Payload remains structured JSON so adding a field to one component kind does
// not force unrelated providers or older clients to understand it.
type AgentActivityComponentDTO struct {
	ID          string                            `json:"id"`
	TurnID      string                            `json:"turnId"`
	ParentID    string                            `json:"parentId,omitempty"`
	Seq         int64                             `json:"seq"`
	Kind        string                            `json:"kind"`
	Status      string                            `json:"status"`
	CreatedAt   time.Time                         `json:"createdAt"`
	UpdatedAt   time.Time                         `json:"updatedAt"`
	CompletedAt *time.Time                        `json:"completedAt,omitempty"`
	Payload     map[string]any                    `json:"payload"`
	Updates     []AgentActivityComponentUpdateDTO `json:"updates,omitempty"`
}

// AgentActivityComponentUpdateDTO is one stable, ordered lifecycle change for
// a component. The owning ledger currently retains lifecycle boundaries rather
// than every live text delta; live deltas continue over the chat stream.
type AgentActivityComponentUpdateDTO struct {
	ID      string         `json:"id"`
	Seq     int            `json:"seq"`
	Kind    string         `json:"kind"`
	At      time.Time      `json:"at"`
	Status  string         `json:"status"`
	Payload map[string]any `json:"payload"`
}

// AgentToolCallDTO is one tool invocation. The payloads themselves are NOT here:
// they are content-addressed and fetched on demand, so a chat with a thousand
// tool calls does not ship megabytes of tool output to render a timeline.
type AgentToolCallDTO struct {
	ID        string                 `json:"id"`
	TurnID    string                 `json:"turnId"`
	Seq       int64                  `json:"seq"`
	Name      string                 `json:"name"`
	Kind      string                 `json:"kind,omitempty"`
	Locations []AgentToolLocationDTO `json:"locations,omitempty"`
	// Target is the file, command or URL the tool acted on, when the provider
	// reports one. Empty is legible; a guess would be wrong.
	Target string `json:"target,omitempty"`
	Status string `json:"status"`
	// Error is a short caption for a failed call. The full failure text is the
	// call's result payload, fetched on demand like any other.
	Error      string `json:"error,omitempty"`
	DurationMS int    `json:"durationMs,omitempty"`
	HasRequest bool   `json:"hasRequest"`
	HasResult  bool   `json:"hasResult"`
	// Diff is a unified diff of an edit call's own change, when known.
	Diff string `json:"diff,omitempty"`
	// SubagentID — see domain.ActivityToolCall's own doc. Set instead of
	// TurnID when this call belongs to a SUBAGENT's own nested activity, not
	// the chat's top-level turn.
	SubagentID string     `json:"subagentId,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	EndedAt    *time.Time `json:"endedAt,omitempty"`
}

type AgentToolLocationDTO struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

type AgentSubagentDTO struct {
	ID        string     `json:"id"`
	TurnID    string     `json:"turnId"`
	Seq       int64      `json:"seq"`
	AgentType string     `json:"agentType,omitempty"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
	// Messages is the subagent's own nested reply history — see
	// domain.ActivitySubagent's own doc.
	Messages []AgentSubagentMessageDTO `json:"messages,omitempty"`
}

type AgentSubagentMessageDTO struct {
	Text string    `json:"text"`
	At   time.Time `json:"at"`
}

// AgentInterruptionDTO is the agent being blocked on, or interrupted by,
// something outside the turn. These are what turn an apparently frozen agent into
// a legible one: a permission wait, a notification, a compaction.
type AgentInterruptionDTO struct {
	ID     string `json:"id"`
	TurnID string `json:"turnId"`
	Seq    int64  `json:"seq"`
	// DisplayOrder — see domain.ActivityInterruption.
	DisplayOrder int64      `json:"displayOrder"`
	Kind         string     `json:"kind"`
	Detail       string     `json:"detail,omitempty"`
	At           time.Time  `json:"at"`
	ResolvedAt   *time.Time `json:"resolvedAt,omitempty"`
}

// AgentChoiceDTO is the agent waiting on a HUMAN DECISION — a tool permission, a
// question it asked, an MCP server's elicitation.
//
// It is the one piece of agent state a user can act on, so it carries enough to be
// drawn as a real prompt rather than a spinner: what kind of answer is wanted,
// what is being asked, and which answers exist.
//
// An answer names this record by ID and an option by its own — see
// POST .../chats/:id/choices/:choiceId/answer.
type AgentChoiceDTO struct {
	ID     string `json:"id"`
	TurnID string `json:"turnId"`
	Seq    int64  `json:"seq"`
	Kind   string `json:"kind"`
	// ToolName is the tool a permission is gating, when there is one.
	ToolName string `json:"toolName,omitempty"`
	Title    string `json:"title,omitempty"`
	Question string `json:"question,omitempty"`
	// Mode is the provider's own word for how it expects to be answered, carried
	// verbatim and never interpreted.
	Mode string `json:"mode,omitempty"`
	// Multi describes the prompt-level Options. A question carries its own, because
	// one payload can mix a single-select question with a multi-select one.
	Multi   bool                   `json:"multi,omitempty"`
	Options []AgentChoiceOptionDTO `json:"options"`
	// Questions is the whole of what a question-kind prompt is asking, one entry
	// per question, and it is what a client must render and answer from.
	//
	// ABSENT is meaningful and is not the same as empty: it says this record
	// predates questions being modelled, and such a prompt is still a single
	// question described by Question and Options. A client falls back to those
	// rather than drawing a card with nothing in it.
	//
	// An answer covering only SOME of these is refused with 400. That is not
	// strictness for its own sake: a partial answer hands the CLI an input covering
	// part of what it asked, and it goes on waiting for the rest with nothing able
	// to send it.
	Questions []AgentChoiceQuestionDTO `json:"questions,omitempty"`
	// Schema is the requested-input schema for a prompt whose answer is a FORM
	// rather than a pick from Options. It is the provider's own JSON, verbatim.
	Schema string `json:"schema,omitempty"`

	Pending bool `json:"pending"`
	// Answerable reports that a relay is holding the provider's gate open for this
	// prompt RIGHT NOW, so answering it here will actually reach the CLI.
	//
	// It is a separate fact from Pending, and both are needed. A prompt whose relay
	// has already timed out is still pending and still worth drawing — the CLI is
	// asking it, at its own terminal — but a button on it would reach nobody, and a
	// surface that cannot tell the two apart offers controls that silently fail.
	Answerable bool       `json:"answerable"`
	At         time.Time  `json:"at"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
	// Resolution says HOW it stopped pending — answered through Crowbar, proceeded
	// (the CLI asked it at its own terminal), refused (nobody answered in time, so
	// the CLI was sent a refusal), or abandoned with the turn.
	Resolution string `json:"resolution,omitempty"`
	// AutoApproved says WHO answered it when Resolution is "answered": policy
	// (true) or a human's own click (false) — currently always false, since
	// Crowbar no longer decides permissions itself (see
	// domain.ActivityChoice.AutoApproved). Never omitted — false is a real
	// answer here, not an absence.
	AutoApproved bool `json:"autoApproved"`
	// AnsweredOptionIDs is which of Options/Questions was actually picked, set
	// only when Resolution is "answered" through Crowbar — see
	// domain.ActivityChoice.AnsweredOptionIDs.
	AnsweredOptionIDs []string `json:"answeredOptionIds,omitempty"`
}

// AgentHookAckDTO is the daemon's reply to a relay that has just delivered a
// hook, and it exists for exactly one instruction: STAY ALIVE.
//
// A vendor CLI holds its own permission gate open for as long as the hook process
// it fired is running, so a relay that exits has already let the CLI's dialog
// through. Await is how the daemon says "this hook opened a question a human can
// answer here" — and it is returned rather than discovered because the relay
// cannot name the prompt: identity is minted daemon-side, from the payload, after
// the chat is resolved.
//
// A hook that opened nothing answerable gets an empty body, and the relay exits
// immediately — byte-identical to its behaviour before this field existed.
//
// Reply is the other: a hook the daemon answers at once. The relay prints it to
// stdout and exits, which is how a message sent mid-turn reaches the running CLI.
type AgentHookAckDTO struct {
	Await *AgentHookAwaitDTO `json:"await,omitempty"`
	Reply string             `json:"reply,omitempty"`
}

// AgentHookAwaitDTO tells a relay what it is waiting on and for how long.
type AgentHookAwaitDTO struct {
	// ChoiceID is Crowbar's identity for the prompt. The relay never interprets it;
	// it carries it so a log line can name what a hook is blocked on.
	ChoiceID string `json:"choiceId"`
	// WaitMS is the daemon's own budget, in milliseconds. The relay bounds its
	// request by it instead of inventing a number, so the budget lives in exactly
	// one place — the provider's descriptor — and stays comfortably inside the
	// timeout injected on the provider's hook registration.
	WaitMS int64 `json:"waitMs"`
}

// AgentHookAnswerDTO is the verdict handed back to a blocked relay.
//
// Stdout is printed VERBATIM, in one write, and is the provider's own JSON — the
// daemon renders it from the descriptor so the relay carries bytes and decides
// nothing. Empty means print nothing, which is the honest answer for a prompt
// that expired or was decided at the terminal.
type AgentHookAnswerDTO struct {
	Stdout string `json:"stdout"`
}

// AgentChoiceQuestionDTO is ONE question inside a prompt, with the options that
// answer it.
//
// Multi is per question and not per prompt because the descriptor maps a
// per-question multi flag, so one prompt can ask "pick one" and "pick any" at
// the same time.
type AgentChoiceQuestionDTO struct {
	// ID is stable within its prompt. A client groups its picks by it; the answer
	// endpoint takes them back as ONE FLAT list of option ids.
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
	// Text is the question as asked, and also the key the provider files the answer
	// under on the way back.
	Text    string                 `json:"text,omitempty"`
	Multi   bool                   `json:"multi,omitempty"`
	Options []AgentChoiceOptionDTO `json:"options"`
}

// AgentChoiceOptionDTO is one answer a prompt will accept. A client renders by
// Kind rather than by Label: allow and deny are Crowbar's own words, because no
// provider enumerates the two answers a permission has by construction.
//
// ID is unique across the WHOLE prompt, not merely within the question that
// offered it, because an answer names its picks in one flat list.
type AgentChoiceOptionDTO struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

// AgentTelemetryDTO is what the provider itself reported about cost and capacity.
//
// Every field is a POINTER because "not reported" and "zero" are different facts
// and a gauge rendering 0% for an unreported value is a lie. A provider that
// reports nothing yields an absent section, and the client draws no gauge.
type AgentTelemetryDTO struct {
	ObservedAt time.Time              `json:"observedAt"`
	Source     string                 `json:"source"`
	Context    *AgentContextUsageDTO  `json:"context,omitempty"`
	RateLimits []AgentRateLimitDTO    `json:"rateLimits,omitempty"`
	Cost       *AgentSessionCostDTO   `json:"cost,omitempty"`
	Model      *AgentModelIdentityDTO `json:"model,omitempty"`
}

// AgentTelemetryDTOFrom maps the provider's usage report onto the wire — the one
// conversion the GET and the pushed `telemetry` frame share.
func AgentTelemetryDTOFrom(report agents.Telemetry) AgentTelemetryDTO {
	out := AgentTelemetryDTO{ObservedAt: report.ObservedAt, Source: report.Source}
	if c := report.Context; c != nil {
		out.Context = &AgentContextUsageDTO{
			CapacityTokens: c.CapacityTokens, UsedTokens: c.UsedTokens,
			UsedPercent: c.UsedPercent, RemainingPercent: c.RemainingPercent,
		}
	}
	for _, w := range report.RateLimits {
		out.RateLimits = append(out.RateLimits, AgentRateLimitDTO{
			ID: w.ID, Label: w.Label, UsedPercent: w.UsedPercent, ResetsAt: w.ResetsAt,
		})
	}
	if c := report.Cost; c != nil {
		out.Cost = &AgentSessionCostDTO{TotalUSD: c.TotalUSD, APIDurationMS: c.APIDurationMS}
	}
	if m := report.Model; m != nil {
		out.Model = &AgentModelIdentityDTO{ID: m.ID, DisplayName: m.DisplayName}
	}
	return out
}

type AgentContextUsageDTO struct {
	CapacityTokens   *int     `json:"capacityTokens,omitempty"`
	UsedTokens       *int     `json:"usedTokens,omitempty"`
	UsedPercent      *float64 `json:"usedPercent,omitempty"`
	RemainingPercent *float64 `json:"remainingPercent,omitempty"`
}

type AgentRateLimitDTO struct {
	ID          string     `json:"id"`
	Label       string     `json:"label,omitempty"`
	UsedPercent *float64   `json:"usedPercent,omitempty"`
	ResetsAt    *time.Time `json:"resetsAt,omitempty"`
}

type AgentSessionCostDTO struct {
	TotalUSD      *float64 `json:"totalUsd,omitempty"`
	APIDurationMS *int     `json:"apiDurationMs,omitempty"`
}

type AgentModelIdentityDTO struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
}
