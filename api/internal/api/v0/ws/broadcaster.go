// Package ws provides the WebSocket broadcaster and HTTP handler for domain
// event streaming over WebSocket connections.
package ws

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// nextID returns a unique string ID for a new client.
func nextID() string { return uuid.NewString() }

const (
	clientBufferSize = 64
	pingInterval     = 30 * time.Second
	writeTimeout     = 10 * time.Second
	pongTimeout      = 60 * time.Second
)

// wsEnvelope is the JSON shape sent to every connected WebSocket client.
type wsEnvelope struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// wsClient represents a single connected WebSocket client registered with a
// Broadcaster.
type wsClient[T any] struct {
	id   string
	ch   chan []byte
	pred func(T) bool // nil predicate means accept all events
}

// Broadcaster fans out typed domain events to all connected WebSocket clients
// whose predicate matches the event. One Broadcaster instance handles one
// entity type (e.g. domain.Task).
type Broadcaster[T any] struct {
	mu      sync.RWMutex
	clients map[string]*wsClient[T]
}

// NewBroadcaster returns an initialised Broadcaster ready to accept
// subscriptions and broadcasts.
func NewBroadcaster[T any]() *Broadcaster[T] {
	return &Broadcaster[T]{
		clients: make(map[string]*wsClient[T]),
	}
}

// Subscribe registers a new WebSocket client with an optional predicate. If
// pred is nil all events are delivered. It returns a read-only channel of
// serialised frames and an unsubscribe function the caller must invoke when the
// client disconnects.
func (b *Broadcaster[T]) Subscribe(pred func(T) bool) (<-chan []byte, func()) {
	cl := &wsClient[T]{
		id:   nextID(),
		ch:   make(chan []byte, clientBufferSize),
		pred: pred,
	}

	b.mu.Lock()
	b.clients[cl.id] = cl
	b.mu.Unlock()

	unsub := func() {
		b.mu.Lock()
		delete(b.clients, cl.id)
		b.mu.Unlock()
	}
	return cl.ch, unsub
}

// Broadcast serialises entity into a typed WS envelope and delivers it
// non-blocking to every subscribed client whose predicate returns true.
// Clients whose buffer is full are skipped (slow clients are dropped).
func (b *Broadcaster[T]) Broadcast(entity T, eventType string) {
	env := wsEnvelope{Type: eventType, Data: entity}
	data, err := json.Marshal(env)
	if err != nil {
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, cl := range b.clients {
		if cl.pred != nil && !cl.pred(entity) {
			continue
		}
		select {
		case cl.ch <- data:
		default:
			// Client channel full — skip to avoid blocking the broadcaster.
		}
	}
}

// writePump reads serialised frames from ch and writes them to conn. It also
// sends periodic pings. It returns when the channel is closed, a write fails,
// or done is closed.
func writePump(conn *websocket.Conn, ch <-chan []byte, done <-chan struct{}) {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(writeTimeout)) //nolint:errcheck
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(writeTimeout)) //nolint:errcheck
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// readPump drains incoming frames and resets the pong deadline so the
// connection stays alive. It returns when the connection closes.
func readPump(conn *websocket.Conn) {
	conn.SetReadDeadline(time.Now().Add(pongTimeout)) //nolint:errcheck
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongTimeout))
	})
	for {
		if _, _, err := conn.NextReader(); err != nil {
			return
		}
	}
}
