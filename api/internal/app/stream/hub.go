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

// Encoder maps a domain value to its wire payload; the API layer supplies it.
type Encoder func(kind stream.EntityKind, v any) (json.RawMessage, error)

// Hub serves every client connection and publishes entity frames to the
// connections holding their scope.
type Hub struct {
	epoch   string
	sources map[stream.ScopeKind]Source
	encode  Encoder

	// mu orders every snapshot against every live frame (index R7).
	mu     sync.Mutex
	seq    uint64
	closed bool
	conns  map[*connection]struct{}
	scopes map[string]map[*connection]struct{}
	refs   *refcount
}

// New builds the hub for one daemon epoch.
func New(
	epoch string,
	sources map[stream.ScopeKind]Source,
	resources map[stream.ScopeKind][]Resource,
	encode Encoder,
) *Hub {
	return &Hub{
		epoch: epoch, sources: sources, encode: encode,
		// A snapshot is stamped with the current seq and the first live frame with seq+1.
		seq:    1,
		conns:  map[*connection]struct{}{},
		scopes: map[string]map[*connection]struct{}{},
		refs:   newRefcount(resources),
	}
}

// Entity publishes data as the current value of (kind, id) to every
// connection holding scope, and returns the version it stamped (0 for none).
func (h *Hub) Entity(scope stream.Scope, kind stream.EntityKind, id string, data any) uint64 {
	raw, err := h.encode(kind, data)
	if err != nil {
		slog.Error("stream: encode an entity", "kind", kind, "id", id, "err", err)
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return 0
	}
	h.seq++
	f := stream.Frame{T: stream.FrameEntity, Scope: scope.String(), Kind: string(kind), ID: id, V: h.seq, Data: raw}
	for c := range h.scopes[f.Scope] {
		if !c.push(f, true) {
			h.dropLocked(c, stream.CloseResync, "resync")
		}
	}
	return h.seq
}

// Serve reads one client's requests until its socket or ctx ends, then
// releases every scope it held.
func (h *Hub) Serve(ctx context.Context, conn Conn) error {
	c := newConnection(conn)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return fmt.Errorf("stream: serve: %w", problem.ErrUnavailable)
	}
	h.conns[c] = struct{}{}
	c.push(stream.Ready(h.epoch), false)
	h.mu.Unlock()
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
	defer h.mu.Unlock()
	h.closed = true
	for c := range h.conns {
		h.dropLocked(c, closeGoingAway, "shutting down")
	}
}

func (h *Hub) handle(ctx context.Context, c *connection, req stream.Request) {
	switch req.Op {
	case stream.OpSub:
		h.sub(ctx, c, req)
	case stream.OpUnsub:
		h.mu.Lock()
		h.releaseLocked(c, req.Scope)
		h.mu.Unlock()
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
	h.mu.Lock()
	defer h.mu.Unlock()
	_, holding := c.scopes[name]
	if h.closed || !holding && len(c.scopes) >= stream.MaxScopes {
		refuse(c, name, problem.New(problem.InvalidArgument, "stream: too many scopes"))
		return
	}
	// A snapshot still queued plus the live frames after it is already current.
	if !c.beginSnapshot(name) {
		return
	}
	start, frames, full := time.Now(), 0, false
	emit := func(f stream.Frame) {
		f.Scope = name
		if f.T == stream.FrameEntity {
			f.V = h.seq
		}
		frames++
		full = full || !c.push(f, false)
	}
	err := src.Attach(ctx, scope.Key, req.Since, emit)
	if err != nil {
		slog.WarnContext(ctx, "stream: snapshot", "scope", name, "err", err)
	}
	if err != nil || full || !c.push(stream.Synced(name), false) {
		// The client reconnects and asks again: a failed snapshot is transient.
		h.dropLocked(c, stream.CloseResync, "resync")
		return
	}
	if !holding {
		c.scopes[name] = scope
		if h.scopes[name] == nil {
			h.scopes[name] = map[*connection]struct{}{}
		}
		h.scopes[name][c] = struct{}{}
		h.refs.hold(scope)
	}
	slog.DebugContext(ctx, "stream: snapshot", "scope", name, "frames", frames, "locked", time.Since(start))
}

// admit reads a sub's scope and its source, answering the client itself when either is wrong.
func (h *Hub) admit(c *connection, raw string) (stream.Scope, Source, bool) {
	if len(raw) > stream.MaxScopeLen {
		c.push(stream.Errored(problem.Of(problem.New(problem.InvalidArgument, "stream: scope too long"))), false)
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
	delete(h.scopes[name], c)
	if len(h.scopes[name]) == 0 {
		delete(h.scopes, name)
	}
	h.refs.release(scope)
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
