package stream

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type following struct{ Source }

func Following(src Source) Source { return following{src} }

type follow struct {
	cancel context.CancelFunc
	synced bool
	since  uint64
}

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
	f := &follow{cancel: cancel, since: h.seq}
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
	h.releaseLocked(c, name)
	if err == nil {
		c.push(stream.Closed(name, stream.ReasonResync, nil), false)
		return
	}
	if problem.CodeOf(err) == problem.Internal {
		slog.WarnContext(ctx, "stream: follow", "scope", name, "err", err)
	}
	refuse(c, name, err)
}

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
	if fr.T == stream.FrameEntity {
		if h.stamps[entityKey(name, fr.Kind, fr.ID)].v > f.since {
			return
		}
		fr.V = h.seq
	}
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
