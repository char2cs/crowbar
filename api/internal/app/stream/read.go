package stream

import (
	"context"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// Entity is one value of a snapshot read outside the hub's lock.
type Entity struct {
	Kind stream.EntityKind
	ID   string
	Data any
}

// Reader is a source whose snapshot is too slow to read under the hub's lock,
// or may itself publish: the hub holds the scope live first, then reads, and
// emits only the entities no live frame carried since.
type Reader interface {
	Source
	Read(ctx context.Context, key string) ([]Entity, error)
}

// pendingRead is one connection's snapshot read in flight for a scope.
type pendingRead struct {
	// since is the seq the scope went live at: a key stamped after it has
	// already reached the client newer than anything the read saw.
	since uint64
}

// subReading emits Attach's frames and registers scope under the lock, then
// reads the rest on a goroutine of its own, so the client's other requests go on.
func (h *Hub) subReading(ctx context.Context, c *connection, scope stream.Scope, src Reader, since int64) bool {
	name := scope.String()
	h.mu.Lock()
	defer h.mu.Unlock()
	_, holding := c.scopes[name]
	if !h.admitLocked(c, name, holding) {
		return holding
	}
	if !h.attachLocked(ctx, c, name, src, scope.Key, since) {
		return false
	}
	if !holding {
		h.registerLocked(c, scope)
	}
	read := &pendingRead{since: h.seq}
	c.reads[name] = read
	h.reads.Add(1)
	go h.read(ctx, c, name, scope.Key, src, read)
	return true
}

// read finishes read's snapshot, unless its hold was let go of meanwhile.
func (h *Hub) read(ctx context.Context, c *connection, name, key string, src Reader, read *pendingRead) {
	defer h.reads.Done()
	defer safego.Recover("stream.read")
	start := time.Now()
	entities, err := src.Read(ctx, key)
	frames := h.encodeRead(entities)

	h.mu.Lock()
	defer h.mu.Unlock()
	if c.reads[name] != read {
		return
	}
	delete(c.reads, name)
	if err != nil {
		h.snapshotFailedLocked(ctx, c, name, err)
		return
	}
	if !h.emitReadLocked(c, name, read, frames) {
		h.snapshotFailedLocked(ctx, c, name, nil)
		return
	}
	slog.DebugContext(ctx, "stream: read snapshot", "scope", name, "entities", len(frames), "took", time.Since(start))
}

// attachLocked queues src's own snapshot frames, stamped with the current seq.
func (h *Hub) attachLocked(ctx context.Context, c *connection, name string, src Source, key string, since int64) bool {
	full := false
	err := src.Attach(ctx, key, since, func(f stream.Frame) {
		f.Scope = name
		if f.T == stream.FrameEntity {
			f.V = h.seq
		}
		full = full || !c.push(f, false)
	})
	if err != nil || full {
		h.snapshotFailedLocked(ctx, c, name, err)
		return false
	}
	return true
}

// encodeRead encodes outside the lock; one value that will not encode is left out.
func (h *Hub) encodeRead(entities []Entity) []stream.Frame {
	frames := make([]stream.Frame, 0, len(entities))
	for _, e := range entities {
		raw, err := h.encode(e.Kind, e.Data)
		if err != nil {
			slog.Error("stream: encode a snapshot entity", "kind", e.Kind, "id", e.ID, "err", err)
			continue
		}
		frames = append(frames, stream.Frame{T: stream.FrameEntity, Kind: string(e.Kind), ID: e.ID, Data: raw})
	}
	return frames
}

// emitReadLocked queues every read frame no live frame superseded, then synced.
func (h *Hub) emitReadLocked(c *connection, name string, read *pendingRead, frames []stream.Frame) bool {
	for _, f := range frames {
		if h.stamps[entityKey(name, f.Kind, f.ID)].v > read.since {
			continue
		}
		f.Scope, f.V = name, h.seq
		if !c.push(f, false) {
			return false
		}
	}
	return c.push(stream.Synced(name), false)
}
