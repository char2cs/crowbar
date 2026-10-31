package ws

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/api/libs"
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

// Handle registers the client BEFORE completing the WebSocket handshake, then
// computes its snapshot OUTSIDE the broadcaster lock and streams live events.
// The snapshot is delivered ahead of live frames via a non-dropping path so it
// is never truncated, and no live event is ever missed (see register/snapshotFor).
//
// Registering ahead of the upgrade is what makes the handshake MEAN something to
// the caller. The 101 is the only edge a client can observe, so it has to be the
// server's statement that this connection is ALREADY in the fan-out set — the
// subscribe-then-mutate flow every caller uses (awaitEntity in the web client,
// dial-then-POST in api/tests) is correct only if it is. Registering after the
// upgrade left a window one goroutine deschedule wide in which an event pushed
// between the 101 hitting the wire and register reaching the map went to a
// subscriber list this connection was not yet in, and no amount of further
// blocking recovers it: the caller waits forever for a frame it already missed.
// On Linux that window cost roughly one flake in five full runs of api/tests,
// always as a 30s stall in whichever test happened to lose the schedule.
func (b *Broadcaster[T]) Handle(
	c *gin.Context,
) {
	// Upgrade would refuse a non-handshake request anyway; refusing it up front
	// keeps a plain GET on a raw .Handle route (chats/ws) from entering the
	// clients map at all.
	if !websocket.IsWebSocketUpgrade(c.Request) {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	snapScope := clientScope(c)
	cl := &filteredClient[T]{client: newClient(), predicate: BuildPredicate(c, b.def)}
	b.register(cl)
	defer b.leave(cl)

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	// The hijacked conn and the clients-map entry must be released even if
	// snapshotFor panics (gin's Recovery would otherwise unwind past the
	// cleanup); a double close of conn is harmless and not logged.
	defer libs.CloseLogged(c.Request.Context(), "ws: close", conn)

	snapshot := b.snapshotFor(cl, snapScope)

	safego.Go("broadcaster.writePump", func() { writePump(conn, cl.client, snapshot) })
	readPump(conn)
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

// leave removes cl from the fan-out and stops its writePump.
func (b *Broadcaster[T]) leave(
	cl *filteredClient[T],
) {
	b.mu.Lock()
	delete(b.clients, cl)
	b.mu.Unlock()
	cl.closeDone()
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
		// (nor crash the daemon — Push runs on a projection goroutine).
		func() {
			defer safego.Recover("broadcaster.Push")
			sendIfMatch(cl, event, data, b.def.CoalesceKey)
		}()
	}
}

func sendIfMatch[T any](
	cl *filteredClient[T],
	event T,
	data []byte,
	coalesceKey func(T) (string, bool),
) {
	if !cl.predicate(event) {
		return
	}
	if coalesceKey != nil {
		if key, ok := coalesceKey(event); ok {
			// Never queued, never disconnects — see client.coalesce and
			// StreamDef.CoalesceKey's own doc comments for why this is safe
			// only for a stream whose values are already "the state so far".
			cl.coalesce(key, data)
			return
		}
	}
	select {
	case cl.send <- data:
	default:
		// Buffer full: this client's writePump is stalled. Dropping the frame
		// would leave it stale; disconnect so it reconnects to a fresh
		// snapshot-on-subscribe. closeDone is idempotent.
		//
		// A stream declaring CoalesceKey never reaches here for the values it
		// claims a key for (see above) — this remains the right policy for
		// everything that does not.
		cl.closeDone()
	}
}
