package dto

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// AgentChatDTO is the wire shape of a Crowbar-owned agentic chat: the workspace it
// belongs to, its title, and when it was created.
//
// It carries NO process facts. activeSegmentId and activeProviderId are gone with
// AgentSegment: a chat does not own the CLI talking to it, so nothing on the chat
// aggregate can answer "is this live" or "which provider". Those are properties of
// the RUNNER pointed at the chat (a live row exists exactly while its PTY does) and
// of the chat's conversation history, and the next task in this series joins them
// back onto this DTO as liveRunnerId / terminalSessionId / activeProviderId
// (derived: the live runner's provider, else the last conversation's). Until then
// this shape is deliberately thin rather than dishonestly full — a field that always
// reads "" is worse than an absent one.
type AgentChatDTO struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	Title       string    `json:"title"`
	CreatedAt   time.Time `json:"createdAt"`
}

// AgentChatDTOFrom converts a persisted AgentChat into its wire shape.
func AgentChatDTOFrom(
	c domain.AgentChat,
) AgentChatDTO {
	return AgentChatDTO{
		ID:          c.ID,
		WorkspaceID: c.WorkspaceID,
		Title:       c.Title,
		CreatedAt:   c.CreatedAt,
	}
}

// AgentChatDTOList converts a slice of AgentChats into wire DTOs, returning a
// non-nil slice so the envelope carries [] rather than null when there are none.
func AgentChatDTOList(
	chats []domain.AgentChat,
) []AgentChatDTO {
	out := make([]AgentChatDTO, 0, len(chats))
	for _, c := range chats {
		out = append(out, AgentChatDTOFrom(c))
	}
	return out
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

// AgentChatEvent is the wire frame pushed on the agent-chat lifecycle
// WebSocket (GET .../workspaces/:wsId/agent/ws/chats): the chat that changed,
// the workspace it belongs to, and the lifecycle kind — chat kinds
// (created/turn_started/turn_stopped/title_set/deleted) and runner kinds
// (started/session_bound/moved/exited), which ride this same workspace-scoped
// feed. It carries no snapshot; the stream is a bare event feed, not a
// full-state resource stream. WorkspaceID both scopes the feed (agentChatDef's
// wsId Filter) and rides along on the wire frame.
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
}
