package dto

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatRuntime is the DERIVED process view of one chat: the runner PLACED on it right
// now, and the conversations it has hosted. Neither is chat state — the chat aggregate
// stores no process facts at all — so both are joined on at read time, off the runner
// projections, by the handler that builds the DTO.
//
// LiveRunner is nil exactly when the chat is DORMANT, and nil is a complete answer, not
// a missing one: a live-runner row exists exactly while its PTY does, so its absence IS
// the liveness verdict. That is why nothing here (and nothing on the wire shapes below)
// carries a status/isLive flag — a second authority on liveness could only drift from
// the process, and that drift is the production bug this refactor exists to delete.
type ChatRuntime struct {
	// LiveRunner is the runner currently pointed at the chat, or nil when none is —
	// i.e. when the chat is dormant because its CLI exited or died with the daemon.
	LiveRunner *agents.Runner

	// Conversations is the chat's append-only history, OLDEST FIRST (so the last
	// element is its last conversation). Empty on a chat no runner has ever spoken
	// into.
	Conversations []agents.ChatConversation

	// TerminalWait is the daemon's standing answer to "is this chat's CLI parked
	// on a modal Crowbar cannot answer?". Derived, never stored, and the zero
	// value — not waiting — is both the common case and the answer for every
	// provider that declares no such prompts.
	TerminalWait domain.AgentTerminalWait
}

// AgentTerminalWaitDTO says a chat's CLI is blocked on a prompt Crowbar has no
// channel to answer, so the only way past it is the terminal.
//
// It is the COMPLEMENT of AgentChoiceDTO. A choice is a prompt that reached
// Crowbar over a hook and can be answered in the chat; this is one that did not
// and cannot, which used to render as nothing at all — no spinner, no message, no
// error, a pane that looked dead over a live process.
//
// Kind is Crowbar's own name for the prompt, and it is OMITTED whenever the daemon
// recognised only that something is up. A client with no kind says exactly that
// ("waiting for input in the terminal") rather than guessing at specifics, and a
// client seeing a kind it does not know does the same — so a daemon that learns a
// new kind never makes an older client say something wrong.
type AgentTerminalWaitDTO struct {
	Kind string `json:"kind,omitempty"`
}

// AgentChatDTO is the wire shape of a Crowbar-owned agentic chat: the workspace it
// belongs to, its title, when it was created — and the three DERIVED runner facts a
// client needs to render and attach to it, joined on from the runner projections
// (they are never stored on the chat).
//
// LiveRunnerID is the whole liveness contract. It names the runner placed on this chat,
// and it is "" exactly when the chat is dormant; a client needs no second call and no
// status field to know whether a pane can attach, because the live row backing it exists
// exactly while the vendor CLI's PTY does. "" here is a MEANINGFUL value (dormant), not
// a placeholder, which is why these fields are always present on the wire rather than
// omitted — the shape is honest either way.
type AgentChatDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Title       string `json:"title"`

	// LiveRunnerID is the id of the runner placed on this chat, or "" when the chat
	// is dormant. This IS the liveness answer — do not look for another.
	LiveRunnerID string `json:"liveRunnerId"`

	// TerminalSessionID is that runner's PTY: the terminal session a chat pane
	// attaches to. Empty exactly when LiveRunnerID is — no runner, nothing to attach.
	TerminalSessionID string `json:"terminalSessionId"`

	// ActiveProviderID is the provider whose CLI is (or last was) talking to this
	// chat: the LIVE runner's provider while the chat is live, and otherwise the
	// provider of its LAST conversation. That fallback is what lets a dormant chat
	// still show the right glyph and dropdown selection, and lets Resume know who to
	// bring back. Empty only on a chat no runner has ever been placed on.
	ActiveProviderID string `json:"activeProviderId"`

	// Working is the chat aggregate's folded busy answer. It is included in the
	// snapshot as well as lifecycle frames so reconnect can reseed truth without
	// guessing from the last event kind.
	Working bool `json:"working"`

	// TerminalWait is present exactly while this chat's CLI is blocked on a
	// terminal-only prompt, and OMITTED otherwise — the common case, and the
	// permanent case for every provider declaring no such prompts, so the wire
	// shape for an unaffected chat is byte-identical to what it was before this
	// existed.
	//
	// It rides the snapshot as well as the lifecycle frame for the same reason
	// Working does: a client that connects while a chat is ALREADY parked missed
	// the frame that said so, and would otherwise show a dead pane until the state
	// happened to change.
	TerminalWait *AgentTerminalWaitDTO `json:"terminalWait,omitempty"`

	// ParentID is the row this chat hangs off in the Chats tree — another chat, a
	// folder, or "" at the panel root — and Order is its dense index within that
	// parent's sibling space, which chats SHARE with chat folders.
	//
	// Both are always present rather than omitted, because "" and 0 are both
	// MEANINGFUL: "" is the panel root and 0 is the first slot. A client that had
	// to tell an absent key from a zero value would be guessing at the two
	// commonest placements there are.
	//
	// A parent that names a CHAT means this row is a THREAD of it and reads that
	// chat's turns; one that names a FOLDER is organisation only. The wire shape
	// does not distinguish them: the client already holds both lists and resolves
	// the id against them.
	ParentID string `json:"parentId"`
	Order    int    `json:"order"`

	// Model and Effort are the chat's sticky choice of what to run its provider
	// CLI as, and they are OMITTED when unset because unset is a real answer:
	// "whatever this provider defaults to", which is not any value the client
	// could render as selected. A picker shows no selection for an absent field,
	// which is exactly right — Crowbar does not know what the default resolves to
	// and will not pretend to.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
}

// AgentChatDTOFrom converts a persisted AgentChat plus its derived runtime into the
// wire shape. The zero ChatRuntime (no live runner, no history) is the honest shape of
// a chat that has never had a runner: every derived field reads "".
func AgentChatDTOFrom(
	c domain.Chat,
	rt ChatRuntime,
) AgentChatDTO {
	out := AgentChatDTO{
		ID:               c.ID,
		WorkspaceID:      c.WorkspaceID,
		Title:            c.Title,
		ActiveProviderID: activeProviderID(rt),
		Working:          c.Working,
		ParentID:         c.ParentID,
		Order:            c.Order,
		Model:            c.Model,
		Effort:           c.Effort,
		CreatedAt:        c.CreatedAt,
	}
	if rt.LiveRunner != nil {
		out.LiveRunnerID = rt.LiveRunner.ID
		out.TerminalSessionID = rt.LiveRunner.TerminalSession
	}
	out.TerminalWait = TerminalWaitDTOFrom(rt.TerminalWait)
	return out
}

// TerminalWaitDTOFrom maps the derived verdict onto the wire, collapsing "not
// waiting" to nil so the field is absent rather than present-and-false. Absence is
// the honest encoding: a chat is not carrying a prompt it does not have.
func TerminalWaitDTOFrom(
	w domain.AgentTerminalWait,
) *AgentTerminalWaitDTO {
	if !w.Waiting {
		return nil
	}
	return &AgentTerminalWaitDTO{Kind: w.Kind}
}

// AgentMessageDTO is one complete hook-derived message in a chat. Sequence is
// Crowbar's per-chat ledger append order; provider-owned transcript identifiers
// and paths never cross this boundary.
type AgentMessageDTO struct {
	Sequence int `json:"sequence"`
	// TurnID is what the activity record attaches tool calls to, so a client can
	// show which tools produced which reply.
	TurnID     string `json:"turnId"`
	Role       string `json:"role"`
	ProviderID string `json:"providerId"`
	Text       string `json:"text"`
	// Effort is the reasoning effort the provider itself reported for this turn
	// (claude's turn_stop carries effort.level), which is NOT the same fact as the
	// chat's requested selection: it is what the CLI says it actually used. Absent
	// when the provider reported none, and never inferred from the request.
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
	ToolCalls     []AgentToolCallDTO     `json:"toolCalls"`
	Subagents     []AgentSubagentDTO     `json:"subagents"`
	Interruptions []AgentInterruptionDTO `json:"interruptions"`
	Choices       []AgentChoiceDTO       `json:"choices"`
}

// AgentToolCallDTO is one tool invocation. The payloads themselves are NOT here:
// they are content-addressed and fetched on demand, so a chat with a thousand
// tool calls does not ship megabytes of tool output to render a timeline.
type AgentToolCallDTO struct {
	ID     string `json:"id"`
	TurnID string `json:"turnId"`
	Seq    int64  `json:"seq"`
	Name   string `json:"name"`
	// Target is the file, command or URL the tool acted on, when the provider
	// reports one. Empty is legible; a guess would be wrong.
	Target string `json:"target,omitempty"`
	Status string `json:"status"`
	// Error is a short caption for a failed call. The full failure text is the
	// call's result payload, fetched on demand like any other.
	Error      string     `json:"error,omitempty"`
	DurationMS int        `json:"durationMs,omitempty"`
	HasRequest bool       `json:"hasRequest"`
	HasResult  bool       `json:"hasResult"`
	StartedAt  time.Time  `json:"startedAt"`
	EndedAt    *time.Time `json:"endedAt,omitempty"`
}

type AgentSubagentDTO struct {
	ID        string     `json:"id"`
	TurnID    string     `json:"turnId"`
	Seq       int64      `json:"seq"`
	AgentType string     `json:"agentType,omitempty"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

// AgentInterruptionDTO is the agent being blocked on, or interrupted by,
// something outside the turn. These are what turn an apparently frozen agent into
// a legible one: a permission wait, a notification, a compaction.
type AgentInterruptionDTO struct {
	ID         string     `json:"id"`
	TurnID     string     `json:"turnId"`
	Seq        int64      `json:"seq"`
	Kind       string     `json:"kind"`
	Detail     string     `json:"detail,omitempty"`
	At         time.Time  `json:"at"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
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
	// Resolution says HOW it stopped pending — answered through Crowbar, observed to
	// have proceeded (someone answered at the terminal), or abandoned with the turn.
	Resolution string `json:"resolution,omitempty"`
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
type AgentHookAckDTO struct {
	Await *AgentHookAwaitDTO `json:"await,omitempty"`
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
// Multi is per question and not per prompt because the provider says so: claude
// carries multiSelect on each entry of its questions array, so one prompt can ask
// "pick one" and "pick any" at the same time.
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

// PromptSubmissionDTO is the wire shape of domain.AgentPromptSubmission — see
// there for what the pair identifies and why a completed model turn is
// deliberately absent from it.
type PromptSubmissionDTO struct {
	RunnerID          string `json:"runnerId"`
	TerminalSessionID string `json:"terminalSessionId"`
}

// SlashCatalogDTO is one ephemeral deterministic provider capability response.
// Completeness is never inferred by Crowbar; it is declared by the provider
// descriptor so partial inventories remain visibly partial.
type SlashCatalogDTO struct {
	ProviderID   string                `json:"providerId"`
	Completeness string                `json:"completeness"`
	Items        []SlashCatalogItemDTO `json:"items"`
	Warnings     []string              `json:"warnings"`
}

type SlashCatalogItemDTO struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Description string `json:"description"`
	InsertText  string `json:"insertText"`
	Source      string `json:"source"`
}

// activeProviderID derives the provider to show for a chat: the live runner's while one
// is placed on it (mid-switch, the incoming runner is already the truth — it outranks a
// history whose last entry still names the outgoing vendor), else the provider of the
// chat's last conversation, else "".
func activeProviderID(
	rt ChatRuntime,
) string {
	if rt.LiveRunner != nil {
		return rt.LiveRunner.ProviderID
	}
	if n := len(rt.Conversations); n > 0 {
		return rt.Conversations[n-1].ProviderID
	}
	return ""
}

// AgentChatDTOList converts a slice of AgentChats into wire DTOs, returning a
// non-nil slice so the envelope carries [] rather than null when there are none.
// runtimes is keyed by chat id; a chat with no entry is rendered from the zero
// ChatRuntime — dormant, no history — which is exactly what a chat missing from both
// runner projections is.
func AgentChatDTOList(
	chats []domain.Chat,
	runtimes map[string]ChatRuntime,
) []AgentChatDTO {
	out := make([]AgentChatDTO, 0, len(chats))
	for _, c := range chats {
		out = append(out, AgentChatDTOFrom(c, runtimes[c.ID]))
	}
	return out
}

// AgentChatDetailDTO is the wire shape of GET .../workspaces/:wsId/chats/:id: the
// chat (with its derived runner facts) plus the conversations it has hosted, oldest
// first. Conversations succeeds the deleted `segments` list: it is what a segment really
// was, minus everything that described a process (no status, no PTY, no runner id), so
// it is pure append-only history and cannot drift from reality. It is always non-nil so
// the envelope carries [] rather than null.
type AgentChatDetailDTO struct {
	AgentChatDTO
	Conversations []agents.ChatConversation `json:"conversations"`
}

// AgentChatDetailDTOFrom composes a chat and its derived runtime into the detail wire
// shape, normalising nil conversations to [] so the envelope never carries null.
func AgentChatDetailDTOFrom(
	c domain.Chat,
	rt ChatRuntime,
) AgentChatDetailDTO {
	convs := rt.Conversations
	if convs == nil {
		convs = []agents.ChatConversation{}
	}
	return AgentChatDetailDTO{
		AgentChatDTO:  AgentChatDTOFrom(c, rt),
		Conversations: convs,
	}
}

// HandoffDTO is the wire shape of GET .../workspaces/:wsId/chats/:id/handoff: the
// assembled handoff blob a freshly spawned provider CLI can be given as prior
// context. Handoff is "" (not omitted) when the chat's ledger has no entries
// yet.
type HandoffDTO struct {
	Handoff string `json:"handoff"`
}

// AgentProviderDTO is the wire shape of domain.AgentProvider (00 agentic-engine
// spec §7.2) — see there for what connected, enabled and mcpEnabled each mean,
// what orders the list, and why efforts arrives already resolved. The catalog is
// workspace-independent but served on the workspace-scoped route for surface
// consistency.
//
// models and efforts are OMITTED rather than sent empty for a provider that
// declares no catalogue: an absent field says the picker does not exist, which an
// empty array would not.
type AgentProviderDTO struct {
	ID           string              `json:"id"`
	DisplayName  string              `json:"displayName"`
	Icon         string              `json:"icon"`
	Connected    bool                `json:"connected"`
	Enabled      bool                `json:"enabled"`
	MCPEnabled   bool                `json:"mcpEnabled"`
	ModelSelect  bool                `json:"modelSelect"`
	EffortSelect bool                `json:"effortSelect"`
	Models       []string            `json:"models,omitempty"`
	Efforts      map[string][]string `json:"efforts,omitempty"`
}

// AgentChatEvent is the wire frame pushed on the agent-chat lifecycle WebSocket
// (GET .../workspaces/:wsId/chats/ws): the thing that changed, the workspace it
// belongs to, and the lifecycle kind — chat kinds (created/turn_started/turn_stopped/
// title_set/placement_set/deleted), runner kinds (started/session_bound/moved/
// displaced/exited), and folder kinds (folder_created/folder_updated/folder_deleted),
// all of which ride this same workspace-scoped feed. It carries no snapshot; the stream is a
// bare event feed, not a full-state resource stream. WorkspaceID both scopes the feed
// (agentChatDef's wsId Filter) and rides along on the wire frame.
//
// ChatID is EMPTY on a `displaced` frame, and that is the frame's whole meaning: Crowbar
// has taken that runner off its chat (an eviction, a provider switch, a chat deleted under
// it) and it now holds nothing. The process may still be alive for a moment — displacement
// asserts nothing about liveness — so a client must not wait for `exited` before letting go
// of it: if the kill failed, `exited` never comes.
type AgentChatEvent struct {
	ChatID      string `json:"chatId"`
	WorkspaceID string `json:"workspaceId"`
	Kind        string `json:"kind"`
	// RunnerID names the vendor-CLI process the frame is about, and is set ONLY on
	// the agent-RUNNER kinds (started/session_bound/moved/exited — see
	// hub.BroadcastAgentRunner), which ride this same workspace-scoped feed rather
	// than a second socket. It is empty on the chat kinds, which are about the chat
	// itself and name no process. A `moved` frame's ChatID is the chat the runner
	// moved INTO, so a client re-points the tab that was following RunnerID.
	RunnerID string `json:"runnerId,omitempty"`
	// FolderID names the CHAT FOLDER a frame is about, and is set ONLY on the
	// folder kinds (folder_created/folder_updated/folder_deleted), which ride this
	// same workspace-scoped feed rather than a second socket — the same reason the
	// runner kinds do.
	//
	// It carries the id and nothing else, deliberately. This stream is a bare event
	// feed, not a full-state resource stream: it has no snapshot, so a client
	// cannot hold folders from it alone and must read the list anyway. Putting the
	// row on the frame would create a second way to learn a folder's placement,
	// and the two would disagree the moment a frame was dropped. A folder frame
	// therefore means exactly "re-read this workspace's folders" — which is also
	// what a reconnect does, so the outage path and the live path repair
	// identically.
	FolderID string `json:"folderId,omitempty"`
	// Working is the chat's folded busy state (domain.Chat.Working) as of this
	// event — the spinner, answered by the server. Set on the CHAT kinds; meaningless
	// on runner kinds, which are about a process and not about a conversation.
	//
	// It is here so the client never re-derives it. `turn_stopped` does NOT mean idle
	// — a CLI that hands work to a background subagent ends its turn and goes quiet
	// waiting for it — so a spinner driven off the kind is wrong precisely when it
	// matters, and a second copy of the fold in TypeScript is a second thing to get
	// wrong. The aggregate folds it once; this carries the answer.
	Working bool `json:"working"`

	// TerminalWait rides the `terminal_wait` kind and nothing else. It is present
	// when the chat's CLI has become blocked on a prompt Crowbar cannot answer, and
	// NIL on the frame that says the block has cleared — so the frame carries the
	// whole answer either way and a client needs no round trip to learn which.
	//
	// Carried on the frame rather than refetched for the same reason Working is:
	// the user is looking at a pane that explains nothing, and a banner that
	// arrives a round trip later is a banner that arrives after they gave up.
	TerminalWait *AgentTerminalWaitDTO `json:"terminalWait,omitempty"`

	// ClientRequestID names one prompt the browser is still holding in its pending
	// queue, and is set ONLY on the prompt_settled kind.
	//
	// The queue resolves an item when the prompt it sent turns up in the ledger as
	// a user message. A provider's own built-in command never produces one — the
	// CLI handles it and announces nothing — so without this frame the item waits
	// forever on evidence that is not coming.
	ClientRequestID string `json:"clientRequestId,omitempty"`

	// Message is an assistant message still being produced, on the message_delta
	// kind and nowhere else.
	//
	// It is deliberately NOT in the ledger. A message that is still growing is not
	// a record of anything yet — it is a view — and writing every increment to
	// durable storage would mean roughly 1.4 writes per second per streaming chat
	// to persist text that is replaced a moment later. So the partial travels on
	// the live feed only, and the ledger gets the message once, when it is done.
	Message *AgentStreamingMessageDTO `json:"message,omitempty"`
}

// AgentStreamingMessageDTO is one assistant message as far as it has been said.
type AgentStreamingMessageDTO struct {
	// ID is the provider's own message identity, so a client can tell a message
	// that is still growing from the next one starting.
	ID string `json:"id"`
	// Text is everything said SO FAR, not the newest increment. A client that
	// missed a frame is therefore correct again on the next one, with no
	// reassembly and no gap detection of its own.
	Text string `json:"text"`
}

// AgentChatKindPromptSettled announces that a prompt Crowbar delivered is OVER
// without having produced a turn, so nothing is still owed on it.
//
// It is emitted at most once per delivery, and only for the deliveries that need
// it: an ordinary prompt is resolved by its own user message arriving in the
// ledger and never reaches this path.
const AgentChatKindPromptSettled = "prompt_settled"

// AgentChatKindMessageDelta announces that an assistant message has grown.
//
// It carries the message SO FAR rather than the increment, so a dropped frame
// self-heals on the next one. It stops when the message is complete: the message
// then exists in the ledger, and the ledger is what the chat reads.
const AgentChatKindMessageDelta = "message_delta"

// AgentChatKindTerminalWait is the lifecycle kind that announces a change in
// whether a chat's CLI is blocked behind a terminal-only prompt.
//
// It is a CHAT kind — it names a conversation, carries no runner id, and is
// emitted only when the verdict MOVES, so a chat parked for an hour produces
// exactly one frame.
const AgentChatKindTerminalWait = "terminal_wait"
