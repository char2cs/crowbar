// Package stream is the daemon's one publisher to clients: one lock, one
// version counter per daemon epoch, one queue per connection.
package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// Source answers a sub for its scope kind. An entity source emits its
// snapshot and returns; emit queues without counting toward the bound.
type Source interface {
	Attach(ctx context.Context, key string, since int64, emit func(stream.Frame)) error
}

// gated is a source that answers per hold, before anything starts, whether this
// client may hold the key at all.
type gated interface {
	Admit(ctx context.Context, key string) error
}

// Quiet serves a scope whose snapshot is empty: its synced says only that it is live.
type Quiet struct{}

// Attach emits nothing.
func (Quiet) Attach(context.Context, string, int64, func(stream.Frame)) error { return nil }

// unordered is a source whose frames carry no version (signals): it attaches
// after its scope is live and outside the hub's lock, so a slow read holds
// back only its own client.
type unordered struct{ Source }

// Unordered marks src as a signal source.
func Unordered(src Source) Source { return unordered{src} }

// Encoder maps a domain value to its wire payload; the API layer supplies it.
type Encoder func(kind stream.EntityKind, v any) (json.RawMessage, error)

// Hub serves every client connection and publishes entity frames to the
// connections holding their scope.
type Hub struct {
	epoch   string
	sources map[stream.ScopeKind]Source
	encode  Encoder

	// mu orders every snapshot against every live frame.
	mu     sync.Mutex
	seq    uint64
	closed bool
	conns  map[*connection]struct{}
	scopes map[string]map[*connection]struct{}
	refs   *refcount
	// stamps is the newest frame each entity key carried.
	stamps map[string]stamp
	// reads are the snapshot reads in flight, which Close ends and waits out.
	reads   sync.WaitGroup
	flights map[string]*readFlight
	life    context.Context
	end     context.CancelFunc
}

// stamp is the newest frame of one entity key: rev is its publisher's own
// version (0 when it has none), v the version the stream stamped, gone whether
// it was a deletion.
type stamp struct {
	rev  int64
	v    uint64
	gone bool
}

func entityKey(scope, kind, id string) string { return scope + "|" + kind + "|" + id }

// New builds the hub for one daemon epoch.
func New(
	epoch string,
	sources map[stream.ScopeKind]Source,
	resources map[stream.ScopeKind][]Resource,
	encode Encoder,
) *Hub {
	life, end := context.WithCancel(context.Background())
	return &Hub{
		epoch: epoch, sources: sources, encode: encode, life: life, end: end,
		flights: map[string]*readFlight{},
		// A snapshot is stamped with the current seq and the first live frame with seq+1.
		seq:    1,
		conns:  map[*connection]struct{}{},
		scopes: map[string]map[*connection]struct{}{},
		refs:   newRefcount(resources),
		stamps: map[string]stamp{},
	}
}

// Entity publishes data as the current value of (kind, id) to every connection
// holding scope and returns the version it stamped (0 for none). A rev older than
// one already published for the key is dropped; rev 0 always publishes.
func (h *Hub) Entity(scope stream.Scope, kind stream.EntityKind, id string, data any, rev int64) uint64 {
	raw, err := h.encode(kind, data)
	if err != nil {
		slog.Error("stream: encode an entity", "kind", kind, "id", id, "err", err)
		return 0
	}
	return h.publishEntity(stream.Frame{T: stream.FrameEntity, Scope: scope.String(), Kind: string(kind), ID: id, Data: raw}, rev)
}

// Gone tells every connection holding scope that (kind, id) no longer exists,
// as of its publisher's rev: no value at that rev or before, or without one,
// brings it back.
func (h *Hub) Gone(scope stream.Scope, kind stream.EntityKind, id string, rev int64) {
	h.publishEntity(stream.Frame{T: stream.FrameEntity, Scope: scope.String(), Kind: string(kind), ID: id, Gone: true}, rev)
}

// Signal tells every connection holding scope news that has no version.
func (h *Hub) Signal(scope stream.Scope, kind string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		slog.Error("stream: encode a signal", "scope", scope.String(), "kind", kind, "err", err)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.closed {
		h.fanOutLocked(stream.Frame{T: stream.FrameSignal, Scope: scope.String(), Kind: kind, Data: raw})
	}
}

func (h *Hub) publishEntity(f stream.Frame, rev int64) uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := entityKey(f.Scope, f.Kind, f.ID)
	held := h.stamps[key]
	if h.closed || rev > 0 && rev < held.rev || held.gone && !f.Gone && rev <= held.rev {
		return 0
	}
	h.seq++
	f.V = h.seq
	h.stamps[key] = stamp{rev: max(rev, held.rev), v: f.V, gone: f.Gone}
	h.fanOutLocked(f)
	return f.V
}

func (h *Hub) fanOutLocked(f stream.Frame) {
	for c := range h.scopes[f.Scope] {
		if !c.push(f, true) {
			h.dropLocked(c, stream.CloseResync, "resync")
		}
	}
}

// Serve reads one client's requests until its socket or ctx ends, then
// releases every scope it held.
func (h *Hub) Serve(ctx context.Context, conn Conn) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c := newConnection(conn)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return fmt.Errorf("stream: serve: %w", problem.ErrUnavailable)
	}
	h.conns[c] = struct{}{}
	c.push(stream.Ready(h.epoch), false)
	h.mu.Unlock()
	defer h.letGo(c)
	defer h.drop(c, closeGoingAway, "")

	go func() {
		defer safego.Recover("stream.write")
		c.write()
	}()
	stop := context.AfterFunc(ctx, func() { c.end(closeGoingAway, "") })
	defer stop()

	for {
		var req stream.Request
		if err := conn.ReadJSON(&req); err != nil {
			if undecodable(err) {
				h.drop(c, closeUnsupportedData, "not a request")
			}
			return fmt.Errorf("stream: read: %w", err)
		}
		h.handle(ctx, c, req)
	}
}

// Close ends every connection; later Serve calls are refused.
func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	for c := range h.conns {
		h.dropLocked(c, closeGoingAway, "shutting down")
	}
	h.mu.Unlock()
	h.end()
	h.reads.Wait()
}

func (h *Hub) handle(ctx context.Context, c *connection, req stream.Request) {
	switch req.Op {
	case stream.OpSub:
		h.sub(ctx, c, req)
	case stream.OpUnsub:
		h.mu.Lock()
		h.releaseLocked(c, req.Scope)
		// Under the hub lock no follow or broadcast can queue another frame of it.
		c.purge(req.Scope)
		h.mu.Unlock()
		h.letGoOf(c, req.Scope)
	default:
		c.push(stream.Errored(problem.Of(problem.New(problem.InvalidArgument, "stream: unknown op"))), false)
	}
}

func undecodable(err error) bool {
	var syntax *json.SyntaxError
	var mistyped *json.UnmarshalTypeError
	return errors.As(err, &syntax) || errors.As(err, &mistyped)
}

func (h *Hub) sub(ctx context.Context, c *connection, req stream.Request) {
	scope, src, ok := h.admit(c, req.Scope)
	if !ok {
		return
	}
	name := scope.String()
	if g, ok := src.(gated); ok {
		if err := g.Admit(ctx, scope.Key); err != nil {
			refuse(c, name, err)
			return
		}
	}
	if err := h.acquire(c, scope); err != nil {
		refuse(c, name, err)
		return
	}
	var held bool
	switch s := src.(type) {
	case unordered:
		held = h.subLive(ctx, c, scope, s.Source, req.Since)
	case following:
		held = h.subFollow(ctx, c, scope, s.Source, req.Since)
	case Reader:
		held = h.subReading(ctx, c, scope, s, req.Since)
	default:
		held = h.subLocked(ctx, c, scope, src, req.Since)
	}
	if !held {
		h.letGoOf(c, name)
	}
}

// subLocked snapshots and registers scope under the lock, so every live frame
// after it carries a higher version. It answers whether c holds scope.
func (h *Hub) subLocked(ctx context.Context, c *connection, scope stream.Scope, src Source, since int64) bool {
	name := scope.String()
	h.mu.Lock()
	defer h.mu.Unlock()
	_, holding := c.scopes[name]
	if !h.admitLocked(c, name, holding) {
		return holding
	}
	start := time.Now()
	if !h.attachLocked(ctx, c, name, src, scope.Key, since) {
		return false
	}
	if !h.syncedLocked(c, name) {
		h.snapshotFailedLocked(ctx, c, name, nil)
		return false
	}
	if !holding {
		h.registerLocked(c, scope)
	}
	slog.DebugContext(ctx, "stream: snapshot", "scope", name, "locked", time.Since(start))
	return true
}

// subLive registers scope first, then snapshots it outside the lock: its
// frames carry no version, so live ones may come before the snapshot's.
func (h *Hub) subLive(ctx context.Context, c *connection, scope stream.Scope, src Source, since int64) bool {
	name := scope.String()
	h.mu.Lock()
	_, holding := c.scopes[name]
	if !h.admitLocked(c, name, holding) {
		h.mu.Unlock()
		return holding
	}
	if !holding {
		h.registerLocked(c, scope)
	}
	h.mu.Unlock()

	full := false
	err := src.Attach(ctx, scope.Key, since, func(f stream.Frame) {
		f.Scope = name
		full = full || !c.push(f, false)
	})
	if err == nil && !full && c.push(stream.Synced(name), false) {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.snapshotFailedLocked(ctx, c, name, err)
	return false
}

// admitLocked answers whether a sub for name snapshots now; a refused one is answered here.
func (h *Hub) admitLocked(c *connection, name string, holding bool) bool {
	if h.closed || !holding && len(c.scopes) >= stream.MaxScopes {
		refuse(c, name, problem.New(problem.InvalidArgument, "stream: too many scopes"))
		return false
	}
	// A held scope's snapshot still queued plus the live frames after it is
	// already current; one let go of since then must snapshot afresh.
	return c.beginSnapshot(name, holding)
}

// snapshotFailedLocked refuses a snapshot of something absent, keeping the
// connection; any other failure is transient, so the client reconnects and
// asks again.
func (h *Hub) snapshotFailedLocked(ctx context.Context, c *connection, name string, err error) {
	if err != nil {
		slog.WarnContext(ctx, "stream: snapshot", "scope", name, "err", err)
	}
	if err != nil && problem.CodeOf(err) == problem.NotFound {
		h.releaseLocked(c, name)
		c.endSnapshot(name)
		refuse(c, name, err)
		return
	}
	h.dropLocked(c, stream.CloseResync, "resync")
}

func (h *Hub) registerLocked(c *connection, scope stream.Scope) {
	name := scope.String()
	c.scopes[name] = scope
	if h.scopes[name] == nil {
		h.scopes[name] = map[*connection]struct{}{}
	}
	if _, held := h.scopes[name][c]; held {
		return
	}
	h.scopes[name][c] = struct{}{}
	h.presenceLocked(scope)
}

// acquire holds what scope's kind runs for c, once per scope. Only c's own
// Serve goroutine calls it, letGoOf and letGo, so c.resources needs no lock.
func (h *Hub) acquire(c *connection, scope stream.Scope) error {
	name := scope.String()
	if _, ok := c.resources[name]; ok {
		return nil
	}
	if len(c.resources) >= stream.MaxScopes {
		return problem.New(problem.InvalidArgument, "stream: too many scopes")
	}
	if err := h.refs.hold(scope); err != nil {
		return fmt.Errorf("stream: start %s: %w", name, err)
	}
	c.resources[name] = scope
	return nil
}

func (h *Hub) letGoOf(c *connection, name string) {
	scope, ok := c.resources[name]
	if !ok {
		return
	}
	delete(c.resources, name)
	h.refs.release(scope)
}

// letGo releases everything c held, once its socket is gone.
func (h *Hub) letGo(c *connection) {
	for name := range c.resources {
		h.letGoOf(c, name)
	}
}

// admit reads a sub's scope and its source, answering the client itself when either is wrong.
func (h *Hub) admit(c *connection, raw string) (stream.Scope, Source, bool) {
	if len(raw) > stream.MaxScopeLen {
		refuse(c, raw, problem.New(problem.InvalidArgument, "stream: scope too long"))
		return stream.Scope{}, nil, false
	}
	scope, err := stream.ParseScope(raw)
	if err != nil {
		refuse(c, raw, err)
		return stream.Scope{}, nil, false
	}
	src, ok := h.sources[scope.Kind]
	if !ok {
		refuse(c, raw, problem.New(problem.InvalidArgument, fmt.Sprintf("stream: no source serves %q", scope.Kind)))
	}
	return scope, src, ok
}

func refuse(c *connection, scope string, err error) {
	p := problem.Of(err)
	c.push(stream.Closed(scope, stream.ReasonRefused, &p), false)
}

func (h *Hub) releaseLocked(c *connection, name string) {
	scope, holding := c.scopes[name]
	if !holding {
		return
	}
	delete(c.scopes, name)
	delete(c.reads, name)
	if f, ok := c.follows[name]; ok {
		f.cancel()
		delete(c.follows, name)
	}
	delete(h.scopes[name], c)
	if len(h.scopes[name]) == 0 {
		delete(h.scopes, name)
	}
	h.presenceLocked(scope)
}

func (h *Hub) drop(c *connection, code int, reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dropLocked(c, code, reason)
}

func (h *Hub) dropLocked(c *connection, code int, reason string) {
	if _, live := h.conns[c]; !live {
		return
	}
	delete(h.conns, c)
	for name := range c.scopes {
		h.releaseLocked(c, name)
	}
	c.end(code, reason)
}
