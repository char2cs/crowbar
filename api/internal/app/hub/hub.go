package hub

import (
	"sync"

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

// BroadcastWorkspace fans a Workspace row out to every subscriber.
func (h *Hub) BroadcastWorkspace(
	ws domain.Workspace,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushWorkspace(ws)
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

var _ WebSocketHub = (*Hub)(nil)
