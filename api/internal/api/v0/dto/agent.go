package dto

import (
	"time"

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
	LiveRunner *domain.AgentRunner

	// Conversations is the chat's append-only history, OLDEST FIRST (so the last
	// element is its last conversation). Empty on a chat no runner has ever spoken
	// into.
	Conversations []domain.ChatConversation
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

	CreatedAt time.Time `json:"createdAt"`
}

// AgentChatDTOFrom converts a persisted AgentChat plus its derived runtime into the
// wire shape. The zero ChatRuntime (no live runner, no history) is the honest shape of
// a chat that has never had a runner: every derived field reads "".
func AgentChatDTOFrom(
	c domain.AgentChat,
	rt ChatRuntime,
) AgentChatDTO {
	out := AgentChatDTO{
		ID:               c.ID,
		WorkspaceID:      c.WorkspaceID,
		Title:            c.Title,
		ActiveProviderID: activeProviderID(rt),
		CreatedAt:        c.CreatedAt,
	}
	if rt.LiveRunner != nil {
		out.LiveRunnerID = rt.LiveRunner.ID
		out.TerminalSessionID = rt.LiveRunner.TerminalSession
	}
	return out
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
	chats []domain.AgentChat,
	runtimes map[string]ChatRuntime,
) []AgentChatDTO {
	out := make([]AgentChatDTO, 0, len(chats))
	for _, c := range chats {
		out = append(out, AgentChatDTOFrom(c, runtimes[c.ID]))
	}
	return out
}

// AgentChatDetailDTO is the wire shape of GET .../workspaces/:wsId/agent/chats/:id: the
// chat (with its derived runner facts) plus the conversations it has hosted, oldest
// first. Conversations succeeds the deleted `segments` list: it is what a segment really
// was, minus everything that described a process (no status, no PTY, no runner id), so
// it is pure append-only history and cannot drift from reality. It is always non-nil so
// the envelope carries [] rather than null.
type AgentChatDetailDTO struct {
	AgentChatDTO
	Conversations []domain.ChatConversation `json:"conversations"`
}

// AgentChatDetailDTOFrom composes a chat and its derived runtime into the detail wire
// shape, normalising nil conversations to [] so the envelope never carries null.
func AgentChatDetailDTOFrom(
	c domain.AgentChat,
	rt ChatRuntime,
) AgentChatDetailDTO {
	convs := rt.Conversations
	if convs == nil {
		convs = []domain.ChatConversation{}
	}
	return AgentChatDetailDTO{
		AgentChatDTO:  AgentChatDTOFrom(c, rt),
		Conversations: convs,
	}
}

// HandoffDTO is the wire shape of GET .../workspaces/:wsId/agent/chats/:id/handoff: the
// assembled handoff blob a freshly spawned provider CLI can be given as prior
// context. Handoff is "" (not omitted) when the chat's ledger has no entries
// yet.
type HandoffDTO struct {
	Handoff string `json:"handoff"`
}

// AgentProviderDTO is the wire shape of one registered agent provider (00
// agentic-engine spec §7.2): the id the FE passes back to create/switch, a
// human display name, and an inline SVG icon (fill="currentColor"). Backed by the
// descriptor enumeration; workspace-independent but served on the workspace-scoped
// route for surface consistency.
type AgentProviderDTO struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Icon        string `json:"icon"`
}

// AgentChatEvent is the wire frame pushed on the agent-chat lifecycle WebSocket
// (GET .../workspaces/:wsId/agent/ws/chats): the chat that changed, the workspace it
// belongs to, and the lifecycle kind — chat kinds (created/turn_started/turn_stopped/
// title_set/deleted) and runner kinds (started/session_bound/moved/displaced/exited),
// which ride this same workspace-scoped feed. It carries no snapshot; the stream is a
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
	// Working is the chat's folded busy state (domain.AgentChat.Working) as of this
	// event — the spinner, answered by the server. Set on the CHAT kinds; meaningless
	// on runner kinds, which are about a process and not about a conversation.
	//
	// It is here so the client never re-derives it. `turn_stopped` does NOT mean idle
	// — a CLI that hands work to a background subagent ends its turn and goes quiet
	// waiting for it — so a spinner driven off the kind is wrong precisely when it
	// matters, and a second copy of the fold in TypeScript is a second thing to get
	// wrong. The aggregate folds it once; this carries the answer.
	Working bool `json:"working"`
}
