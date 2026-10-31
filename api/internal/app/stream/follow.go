package stream

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// following is a ledger source: Attach replays, emits synced, then keeps
// emitting until its ctx ends. The hub runs it on a goroutine of its own,
// outside the lock; the ledger's seq orders its frames, not the hub's.
type following struct{ Source }

// Following marks src as a ledger source.
func Following(src Source) Source { return following{src} }

// follow is one connection's hold on a ledger scope.
type follow struct {
	cancel context.CancelFunc
	synced bool
}

// subFollow starts following scope for c, unless c already does: that hold is current.
func (h *Hub) subFollow(ctx context.Context, c *connection, scope stream.Scope, src Source, since int64) bool {
	name := scope.String()
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, holding := c.scopes[name]; holding {
		return true
	}
	if h.closed || len(c.scopes) >= stream.MaxScopes {
		refuse(c, name, problem.New(problem.InvalidArgument, "stream: too many scopes"))
		return false
	}
	fctx, cancel := context.WithCancel(ctx)
	f := &follow{cancel: cancel}
	c.follows[name] = f
	h.registerLocked(c, scope)
	go h.runFollow(fctx, c, scope, src, since, f)
	return true
}

func (h *Hub) runFollow(ctx context.Context, c *connection, scope stream.Scope, src Source, since int64, f *follow) {
	defer safego.Recover("stream.follow")
	name := scope.String()
	err := src.Attach(ctx, scope.Key, since, func(fr stream.Frame) { h.followed(ctx, c, name, f, fr) })
	if ctx.Err() != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if c.follows[name] != f {
		return
	}
	switch {
	case err != nil && problem.CodeOf(err) == problem.NotFound:
		h.releaseLocked(c, name)
		refuse(c, name, err)
	case err != nil:
		slog.WarnContext(ctx, "stream: follow", "scope", name, "err", err)
		h.dropLocked(c, stream.CloseResync, "resync")
	default:
		// The ledger ended without saying why: the client asks again from its seq.
		h.releaseLocked(c, name)
		c.push(stream.Closed(name, stream.ReasonResync, nil), false)
	}
}

// followed queues one frame of f's hold, if it still holds. The replay waits
// for room in the queue (index R8); only live frames count toward the bound.
func (h *Hub) followed(ctx context.Context, c *connection, name string, f *follow, fr stream.Frame) {
	if !f.synced && !c.await(ctx) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if c.follows[name] != f {
		return
	}
	fr.Scope = name
	if fr.T == stream.FrameClosed {
		h.releaseLocked(c, name)
		c.push(fr, false)
		return
	}
	if !c.push(fr, f.synced) {
		h.dropLocked(c, stream.CloseResync, "resync")
		return
	}
	f.synced = f.synced || fr.T == stream.FrameSynced
}
