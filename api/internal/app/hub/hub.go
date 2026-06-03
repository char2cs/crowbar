package hub

import (
	"sync"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Hub fans out domain broadcasts to all registered Subscribers.
// It implements WebSocketHub so the app layer can broadcast domain events
// without depending on the API layer.
type Hub struct {
	mu          sync.RWMutex
	subscribers []Subscriber
}

// NewHub returns a ready-to-use Hub.
func NewHub() *Hub {
	return &Hub{}
}

// Register adds s to the set of active subscribers.
// Safe to call concurrently.
func (h *Hub) Register(
	s Subscriber,
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subscribers = append(h.subscribers, s)
}

// Unregister removes s from the set of active subscribers.
// Safe to call concurrently.
func (h *Hub) Unregister(
	s Subscriber,
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	next := make([]Subscriber, 0, len(h.subscribers))
	for _, sub := range h.subscribers {
		if sub != s {
			next = append(next, sub)
		}
	}
	h.subscribers = next
}

// BroadcastTask fans out t to every registered Subscriber.
func (h *Hub) BroadcastTask(
	t domain.Task,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushTask(t)
	}
}

// BroadcastAgentRun fans out r to every registered Subscriber.
func (h *Hub) BroadcastAgentRun(
	r domain.AgentRun,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushAgentRun(r)
	}
}

// BroadcastKanbanItem fans out i to every registered Subscriber.
func (h *Hub) BroadcastKanbanItem(
	i domain.KanbanItem,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushKanbanItem(i)
	}
}

// BroadcastReviewThread fans out t to every registered Subscriber.
func (h *Hub) BroadcastReviewThread(
	t domain.ReviewThread,
) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushReviewThread(t)
	}
}
