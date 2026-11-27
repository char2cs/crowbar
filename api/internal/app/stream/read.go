package stream

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type Entity struct {
	Kind stream.EntityKind
	ID   string
	Data any
}

type Snapshot struct {
	Entities []Entity
	Unread   []string
}

type Reader interface {
	Source
	Read(ctx context.Context, key string) (Snapshot, error)
}

type pendingRead struct {
	since uint64
}

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
	f := h.flights[name]
	if f == nil {
		f = &readFlight{}
		h.flights[name] = f
	}
	f.next = append(f.next, joiner{c: c, read: read})
	if !f.running {
		f.running = true
		h.reads.Add(1)
		go h.fly(context.WithoutCancel(ctx), name, scope.Key, src, f)
	}
	return true
}

type readFlight struct {
	running bool
	next    []joiner
}

type joiner struct {
	c    *connection
	read *pendingRead
}

func (h *Hub) fly(ctx context.Context, name, key string, src Reader, f *readFlight) {
	defer h.reads.Done()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(h.life, cancel) //nolint:contextcheck // h.life is the hub's lifetime: Close ends a read no tab will wait for
	defer stop()
	for {
		batch := h.takeBatch(name, f)
		if batch == nil {
			return
		}
		start := time.Now()
		snap, err := readSafely(ctx, src, key)
		frames := h.encodeRead(snap.Entities)
		h.mu.Lock()
		for _, j := range batch {
			h.answerLocked(ctx, j, name, snap.Unread, frames, err)
		}
		h.mu.Unlock()
		slog.DebugContext(ctx, "stream: read snapshot", "scope", name, "joiners", len(batch), "entities", len(frames), "took", time.Since(start))
	}
}

func (h *Hub) takeBatch(name string, f *readFlight) []joiner {
	h.mu.Lock()
	defer h.mu.Unlock()
	var batch []joiner
	for _, j := range f.next {
		if j.c.reads[name] == j.read {
			batch = append(batch, j)
		}
	}
	f.next = nil
	if len(batch) == 0 {
		f.running = false
		delete(h.flights, name)
		return nil
	}
	return batch
}

func readSafely(ctx context.Context, src Reader, key string) (snap Snapshot, err error) {
	defer safego.RecoverFn("stream.read", func(r any) { err = fmt.Errorf("stream: read panicked: %v", r) })
	return src.Read(ctx, key)
}

func (h *Hub) answerLocked(ctx context.Context, j joiner, name string, unread []string, frames []stream.Frame, err error) {
	if j.c.reads[name] != j.read {
		return
	}
	delete(j.c.reads, name)
	if err != nil {
		h.snapshotFailedLocked(ctx, j.c, name, err)
		return
	}
	if !h.emitReadLocked(j.c, name, j.read, frames, unread) {
		h.snapshotFailedLocked(ctx, j.c, name, nil)
	}
}

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

func (h *Hub) emitReadLocked(c *connection, name string, read *pendingRead, frames []stream.Frame, unread []string) bool {
	for _, f := range frames {
		if h.stamps[entityKey(name, f.Kind, f.ID)].v > read.since {
			continue
		}
		f.Scope, f.V = name, h.seq
		if !c.push(f, false) {
			return false
		}
	}
	return h.syncedLocked(c, name, unread...)
}
