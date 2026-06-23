package ws

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/core/safego"
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
	// regCount is a buffered semaphore channel: one token is sent per registration.
	// Test helpers drain exactly n tokens to confirm n clients are registered.
	regCount chan struct{}
}

// NewBroadcaster builds a Broadcaster from a StreamDef.
func NewBroadcaster[T any](
	def StreamDef[T],
) *Broadcaster[T] {
	return &Broadcaster[T]{
		def:        def,
		clients:    make(map[*filteredClient[T]]struct{}),
		registered: make(chan struct{}),
		regCount:   make(chan struct{}, 1024),
	}
}

// WaitRegistered blocks until at least one client has registered. Test-only.
func (b *Broadcaster[T]) WaitRegistered() {
	<-b.registered
}

// WaitNRegistered blocks until n clients have registered. Test-only.
// Unlike WaitRegistered (sync.Once), this counts every registration individually,
// so it is safe to call after multiple Dial calls on the same Env.
func (b *Broadcaster[T]) WaitNRegistered(
	n int,
) {
	for i := 0; i < n; i++ {
		<-b.regCount
	}
}

// Handle upgrades the request to a WebSocket, registers the client, computes its
// snapshot OUTSIDE the broadcaster lock, then streams live events. The snapshot
// is delivered ahead of live frames via a non-dropping path so it is never
// truncated, and no live event is ever missed (see register/snapshotFor).
func (b *Broadcaster[T]) Handle(
	c *gin.Context,
) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	// The hijacked conn, the clients-map entry, and the watcher/LSP refcount must
	// all be released even if snapshotFor/onSubscribe panics (gin's Recovery would
	// otherwise unwind past the cleanup, leaking an FD, a dead map entry that every
	// future Push iterates, and a watcher/LSP refcount). Defers make cleanup
	// unconditional; double-close of conn is harmless (the err is ignored).
	defer func() { _ = conn.Close() }()

	predicate := BuildPredicate(c, b.def)
	cl := &filteredClient[T]{client: newClient(), predicate: predicate}

	scope := b.scopeKey(c)
	snapScope := clientScope(c)

	b.register(cl)
	defer b.remove(cl)

	snapshot := b.snapshotFor(cl, snapScope)
	b.onSubscribe(scope)
	defer b.onUnsubscribe(scope)

	safego.Go("broadcaster.writePump", func() { writePump(conn, cl.client, snapshot) })
	readPump(conn)
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

// register adds cl to the clients map and signals registration. It runs entirely
// under b.mu but does NOT compute the snapshot: from this point cl receives every
// live frame into cl.send. The snapshot is computed afterwards in snapshotFor,
// outside the lock, so any live event between registration and snapshot is queued
// into cl.send AND captured by the snapshot — double-delivery is harmless because
// every snapshot topic is an idempotent full-state replace, and no live event is
// ever missed (no gap).
func (b *Broadcaster[T]) register(
	cl *filteredClient[T],
) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[cl] = struct{}{}
	b.once.Do(func() { close(b.registered) })
	// Non-blocking send: regCount is buffered (1024). WaitNRegistered drains tokens.
	select {
	case b.regCount <- struct{}{}:
	default:
	}
}

func (b *Broadcaster[T]) remove(
	cl *filteredClient[T],
) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.clients, cl)
	close(cl.done)
}

// snapshotFor computes cl's snapshot OUTSIDE the broadcaster lock (b.def.Snapshot
// performs git/DB reads, so holding b.mu here would block every live Push for the
// duration of one client's connect). It returns the matching, serialized frames as
// an unbounded slice so the snapshot is never truncated by the bounded cl.send
// buffer; writePump flushes these ahead of any live frame.
func (b *Broadcaster[T]) snapshotFor(
	cl *filteredClient[T],
	scope string,
) [][]byte {
	if b.def.Snapshot == nil {
		return nil
	}
	items := b.def.Snapshot(scope)
	frames := make([][]byte, 0, len(items))
	for _, item := range items {
		if !cl.predicate(item) {
			continue
		}
		data, err := b.def.Serialize(item)
		if err != nil {
			continue
		}
		frames = append(frames, data)
	}
	return frames
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
		// A panic in one client's predicate must not abort the fan-out to the rest
		// (nor crash the daemon — Push runs on a watcher/projection goroutine).
		func() {
			defer safego.Recover("broadcaster.Push")
			sendIfMatch(cl, event, data)
		}()
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
