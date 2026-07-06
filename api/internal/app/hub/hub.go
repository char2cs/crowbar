package hub

import (
	"sync"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
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

// BroadcastThread fans a ThreadDTO out to every subscriber (spec §5).
func (h *Hub) BroadcastThread(
	t dto.ThreadDTO,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushThread(t)
	}
}

// BroadcastTerminalSession fans a TerminalSessionDTO out to every subscriber
// (spec §5).
func (h *Hub) BroadcastTerminalSession(
	s dto.TerminalSessionDTO,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, sub := range h.subscribers {
		sub.PushTerminalSession(s)
	}
}

// BroadcastGit fans a GitStatus out to every subscriber (Class B, 03 §2).
func (h *Hub) BroadcastGit(
	wsID string,
	status gitdomain.GitStatus,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushGit(wsID, status)
	}
}

// BroadcastFile fans a FileChangeEvent out to every subscriber (Class B, 03 §2).
func (h *Hub) BroadcastFile(
	evt domain.FileChangeEvent,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushFile(evt)
	}
}

// BroadcastAgentChat fans an agent-chat lifecycle event (spawn/bound/focus/
// registered/switched/turn_stopped) out to every subscriber.
func (h *Hub) BroadcastAgentChat(
	chatID string,
	kind string,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentChat(chatID, kind)
	}
}

var _ WebSocketHub = (*Hub)(nil)
