package dto

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// AgentChatDTO is the wire shape of a Crowbar-owned agentic chat (00
// agentic-engine spec §6): the workspace it belongs to, the id of its currently
// active provider segment, and its creation timestamp.
type AgentChatDTO struct {
	ID              string `json:"id"`
	WorkspaceID     string `json:"workspaceId"`
	Title           string `json:"title"`
	ActiveSegmentID string `json:"activeSegmentId"`
	// ActiveProviderID is the providerId of the currently active segment (00
	// agentic-engine spec §7.3), derived from the embedded segments so the chat
	// list can map a row to its provider glyph without N detail fetches. Empty
	// when the chat has no active segment.
	ActiveProviderID string    `json:"activeProviderId"`
	CreatedAt        time.Time `json:"createdAt"`
}

// AgentChatDTOFrom converts a persisted AgentChat into its wire shape.
func AgentChatDTOFrom(
	c domain.AgentChat,
) AgentChatDTO {
	return AgentChatDTO{
		ID:               c.ID,
		WorkspaceID:      c.WorkspaceID,
		Title:            c.Title,
		ActiveSegmentID:  c.ActiveSegmentID,
		ActiveProviderID: activeProviderID(c),
		CreatedAt:        c.CreatedAt,
	}
}

// activeProviderID returns the providerId of c's active segment (the one whose
// id is c.ActiveSegmentID), or "" when there is none.
func activeProviderID(
	c domain.AgentChat,
) string {
	for _, s := range c.Segments {
		if s.ID == c.ActiveSegmentID {
			return s.ProviderID
		}
	}
	return ""
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

// AgentChatDetailDTO is the wire shape of GET .../workspaces/:wsId/agent/chats/:id: the chat plus
// its ordered segment history (oldest first). Segments is always non-nil so the
// envelope carries [] rather than null when the chat has no segments yet.
type AgentChatDetailDTO struct {
	AgentChatDTO
	Segments []domain.AgentSegment `json:"segments"`
}

// AgentChatDetailDTOFrom composes a chat and its segments into the detail wire
// shape, normalising a nil segment slice to [] so the envelope never carries
// null.
func AgentChatDetailDTOFrom(
	c domain.AgentChat,
	segs []domain.AgentSegment,
) AgentChatDetailDTO {
	if segs == nil {
		segs = []domain.AgentSegment{}
	}
	return AgentChatDetailDTO{
		AgentChatDTO: AgentChatDTOFrom(c),
		Segments:     segs,
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

// AgentChatEvent is the wire frame pushed on the agent-chat lifecycle
// WebSocket (GET .../workspaces/:wsId/agent/ws/chats): the chat that changed,
// the workspace it belongs to, and the lifecycle kind (created/segment_opened/
// segment_ended/session_bound/turn_started/turn_stopped/title_set/deleted —
// 00 agentic-engine spec §7). It carries no snapshot; the stream is a bare
// event feed, not a full-state resource stream. WorkspaceID both scopes the
// feed (agentChatDef's wsId Filter, Task 3) and rides along on the wire frame.
type AgentChatEvent struct {
	ChatID      string `json:"chatId"`
	WorkspaceID string `json:"workspaceId"`
	Kind        string `json:"kind"`
}
