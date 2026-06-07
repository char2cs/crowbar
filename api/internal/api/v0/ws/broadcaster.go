package ws

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

type filteredClient[T any] struct {
	*client
	predicate func(T) bool
}

// Broadcaster fans a stream of T out to filtered WebSocket clients (03 §1).
type Broadcaster[T any] struct {
	def        StreamDef[T]
	mu         sync.RWMutex
	clients    map[*filteredClient[T]]struct{}
	registered chan struct{}
	once       sync.Once
}

// NewBroadcaster builds a Broadcaster from a StreamDef.
func NewBroadcaster[T any](
	def StreamDef[T],
) *Broadcaster[T] {
	return &Broadcaster[T]{
		def:        def,
		clients:    make(map[*filteredClient[T]]struct{}),
		registered: make(chan struct{}),
	}
}

// WaitRegistered blocks until at least one client has registered. Test-only.
func (b *Broadcaster[T]) WaitRegistered() {
	<-b.registered
}

// Handle upgrades the request to a WebSocket, registers the client (delivering
// the snapshot atomically under the registration lock), then streams live events.
func (b *Broadcaster[T]) Handle(
	c *gin.Context,
) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	predicate := BuildPredicate(c, b.def)
	cl := &filteredClient[T]{client: newClient(), predicate: predicate}

	scope := b.scopeKey(c)

	b.register(cl)
	b.onSubscribe(scope)
	go writePump(conn, cl.client)
	readPump(conn)
	b.remove(cl)
	b.onUnsubscribe(scope)
}

func (b *Broadcaster[T]) scopeKey(
	c *gin.Context,
) string {
	if b.def.ScopeKey == nil {
		return ""
	}
	return b.def.ScopeKey(c)
}

func (b *Broadcaster[T]) onSubscribe(
	scope string,
) {
	if b.def.OnSubscribe == nil {
		return
	}
	b.def.OnSubscribe(scope)
}

func (b *Broadcaster[T]) onUnsubscribe(
	scope string,
) {
	if b.def.OnUnsubscribe == nil {
		return
	}
	b.def.OnUnsubscribe(scope)
}

func (b *Broadcaster[T]) register(
	cl *filteredClient[T],
) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[cl] = struct{}{}
	b.pushSnapshot(cl)
	b.once.Do(func() { close(b.registered) })
}

func (b *Broadcaster[T]) remove(
	cl *filteredClient[T],
) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, cl)
	close(cl.done)
}

func (b *Broadcaster[T]) pushSnapshot(
	cl *filteredClient[T],
) {
	if b.def.Snapshot == nil {
		return
	}
	for _, item := range b.def.Snapshot() {
		b.serializeAndSend(cl, item)
	}
}

func (b *Broadcaster[T]) serializeAndSend(
	cl *filteredClient[T],
	item T,
) {
	data, err := b.def.Serialize(item)
	if err != nil {
		return
	}
	sendIfMatch(cl, item, data)
}

// Push serializes the event once and delivers it to every matching client.
func (b *Broadcaster[T]) Push(
	event T,
) {
	data, err := b.def.Serialize(event)
	if err != nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for cl := range b.clients {
		sendIfMatch(cl, event, data)
	}
}

func sendIfMatch[T any](
	cl *filteredClient[T],
	event T,
	data []byte,
) {
	if !cl.predicate(event) {
		return
	}
	select {
	case cl.send <- data:
	default:
	}
}
