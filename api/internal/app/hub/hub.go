package hub

import (
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
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

// BroadcastChat fans a ChatStatusEvent out to every subscriber.
func (h *Hub) BroadcastChat(
	evt ChatStatusEvent,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushChat(evt)
	}
}

var _ WebSocketHub = (*Hub)(nil)
