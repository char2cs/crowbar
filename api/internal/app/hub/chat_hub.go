package hub

import (
	"sync"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
)

// ChatHub coordinates bidirectional chat between ACP agent sessions and
// WebSocket clients. Defined in the app layer so the engine and MCP layers
// can depend on it without importing the API presentation layer.
type ChatHub interface {
	RegisterSession(taskID string) (input <-chan string, publish func(agent.ChatFrame), unregister func())
	Subscribe(taskID string) (frames <-chan agent.ChatFrame, unsubscribe func())
	Forward(taskID string, content string)
	Publish(taskID string, frame agent.ChatFrame)
}

type chatSession struct {
	input   chan string
	publish func(agent.ChatFrame)
}

type chatSubscriber struct {
	ch chan agent.ChatFrame
}

type chatHub struct {
	mu       sync.RWMutex
	sessions map[string]*chatSession
	subs     map[string][]*chatSubscriber
}

// NewChatHub returns a ready-to-use ChatHub.
func NewChatHub() ChatHub {
	return &chatHub{
		sessions: make(map[string]*chatSession),
		subs:     make(map[string][]*chatSubscriber),
	}
}

func (h *chatHub) RegisterSession(taskID string) (input <-chan string, publish func(agent.ChatFrame), unregister func()) {
	in := make(chan string, 32)

	pub := func(frame agent.ChatFrame) {
		h.mu.RLock()
		defer h.mu.RUnlock()
		for _, sub := range h.subs[taskID] {
			select {
			case sub.ch <- frame:
			default:
			}
		}
	}

	s := &chatSession{input: in, publish: pub}

	h.mu.Lock()
	h.sessions[taskID] = s
	h.mu.Unlock()

	unreg := func() {
		h.mu.Lock()
		delete(h.sessions, taskID)
		h.mu.Unlock()
	}

	return in, pub, unreg
}

func (h *chatHub) Subscribe(taskID string) (frames <-chan agent.ChatFrame, unsubscribe func()) {
	ch := make(chan agent.ChatFrame, 64)
	sub := &chatSubscriber{ch: ch}

	h.mu.Lock()
	h.subs[taskID] = append(h.subs[taskID], sub)
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		list := h.subs[taskID]
		next := make([]*chatSubscriber, 0, len(list))
		for _, s := range list {
			if s != sub {
				next = append(next, s)
			}
		}
		h.subs[taskID] = next
	}

	return ch, unsub
}

func (h *chatHub) Forward(taskID string, content string) {
	h.mu.RLock()
	s, ok := h.sessions[taskID]
	h.mu.RUnlock()
	if !ok {
		return
	}
	select {
	case s.input <- content:
	default:
	}
}

func (h *chatHub) Publish(taskID string, frame agent.ChatFrame) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, sub := range h.subs[taskID] {
		select {
		case sub.ch <- frame:
		default:
		}
	}
}
