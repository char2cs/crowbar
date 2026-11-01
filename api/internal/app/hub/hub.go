package hub

import (
	"sync"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Hub fans out domain broadcasts to all registered Subscribers. It implements
// WebSocketHub so the app layer can broadcast through it.
type Hub struct {
	mu          sync.RWMutex
	subscribers []Subscriber
}

// NewHub constructs an empty Hub.
func NewHub() *Hub {
	return &Hub{}
}

// Register adds a Subscriber to the fan-out set.
func (h *Hub) Register(
	s Subscriber,
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subscribers = append(h.subscribers, s)
}

// BroadcastProject fans a ProjectDTO out to every subscriber (spec §5).
func (h *Hub) BroadcastProject(
	p dto.ProjectDTO,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushProject(p)
	}
}

// BroadcastRepo fans a RepoDTO out to every subscriber (spec §5).
func (h *Hub) BroadcastRepo(
	r dto.RepoDTO,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushRepo(r)
	}
}

// BroadcastWorkspace fans a WorkspaceDTO out to every subscriber (spec §5). The
// merge-eligibility overlay is resolved by the producer before this call.
func (h *Hub) BroadcastWorkspace(
	w dto.WorkspaceDTO,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushWorkspace(w)
	}
}

// BroadcastAgentChatEvent fans one chat snapshot frame out on the chat feed —
// every chat and runner lifecycle change, carrying the chat's full versioned
// snapshot (see usecases/chat/internal/snapshot).
func (h *Hub) BroadcastAgentChatEvent(ev dto.AgentChatEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChatEvent(ev)
	}
}

// BroadcastAgentChatTelemetry fans the provider's newest usage report for a
// chat out on the same workspace-scoped feed, so the gauge moves when the
// provider reports instead of on a client poll.
func (h *Hub) BroadcastAgentChatTelemetry(
	chatID string,
	workspaceID string,
	report agents.Telemetry,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChatTelemetry(chatID, workspaceID, report)
	}
}

// BroadcastAgentChatFolder fans a CHAT FOLDER lifecycle event
// (folder_created/folder_updated/folder_deleted) out on the SAME workspace-scoped
// agent-chat WebSocket as BroadcastAgentChat. A chat folder is a plain GORM row
// with no aggregate projection to ride, so the mutating handler calls this
// itself right after the write — exactly as the sidebar's folder handlers call
// BroadcastFolder.
//
// A second socket for folders would buy nothing and would have to be kept in
// ORDER with the first: one gesture moves both kinds (they share a sibling
// space), so a folder frame and the chat frames its densify caused have to
// arrive in a sequence the client can reconcile.
//
// The frame names the folder and nothing more. The Chats feed carries no
// snapshot, so a client cannot hold folders from it alone; putting the row on
// the frame would create a second way to learn a placement, and the two would
// disagree the first time a frame was dropped.
func (h *Hub) BroadcastAgentChatFolder(
	folderID string,
	workspaceID string,
	kind string,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChatFolder(folderID, workspaceID, kind)
	}
}

var _ WebSocketHub = (*Hub)(nil)
