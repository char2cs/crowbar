package hub

import "context"

type Event struct {
	ID   string `json:"id,omitempty"`
	Type string `json:"type"`
	Data any    `json:"data"`
}

type Hub struct {
	broadcast  chan Event
	register   chan chan Event
	unregister chan chan Event
	clients    map[chan Event]struct{}
}

func New() *Hub {
	return &Hub{
		broadcast:  make(chan Event, 256),
		register:   make(chan chan Event),
		unregister: make(chan chan Event),
		clients:    make(map[chan Event]struct{}),
	}
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = struct{}{}
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client)
			}
		case event := <-h.broadcast:
			for client := range h.clients {
				select {
				case client <- event:
				default:
					delete(h.clients, client)
					close(client)
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (h *Hub) Register() chan Event {
	ch := make(chan Event, 10)
	h.register <- ch
	return ch
}

func (h *Hub) Unregister(ch chan Event) {
	h.unregister <- ch
}

func (h *Hub) Broadcast(e Event) {
	h.broadcast <- e
}
