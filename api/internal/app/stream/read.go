package stream

import (
	"context"
	"fmt"
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

// Snapshot is what a Reader read: its entities, and the parts of its scope it
// could not read, whose entities the client keeps rather than counts absent.
type Snapshot struct {
	Entities []Entity
	Unread   []string
}

// Reader is a source whose snapshot is too slow to read under the hub's lock,
// or may itself publish: the hub holds the scope live first, then reads, and
// emits only the entities no live frame carried since.
type Reader interface {
	Source
	Read(ctx context.Context, key string) (Snapshot, error)
}

// pendingRead is one connection's snapshot read in flight for a scope.
type pendingRead struct {
	// since is the seq the scope went live at: a key stamped after it has
	// already reached the client newer than anything the read saw.
	since uint64
}

// subReading emits Attach's frames and registers scope under the lock, then
// joins the scope's next read, which runs on a goroutine of its own so the
// client's other requests go on.
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

// readFlight is one scope's reads, shared by every connection that asks for it.
// A sub arriving while a read runs joins the next: that read starts after the
// sub went live, so the live frames it missed are older than what it reads.
type readFlight struct {
	running bool
	next    []joiner
}

type joiner struct {
	c    *connection
	read *pendingRead
}

// fly reads for each batch of joiners until none is waiting.
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

// takeBatch claims the joiners still waiting on f's next read, retiring f when none is.
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

// readSafely is src's read, a panic in it failing the read rather than the flight.
func readSafely(ctx context.Context, src Reader, key string) (snap Snapshot, err error) {
	defer safego.RecoverFn("stream.read", func(r any) { err = fmt.Errorf("stream: read panicked: %v", r) })
	return src.Read(ctx, key)
}

// answerLocked finishes j's snapshot, unless its hold was let go of meanwhile.
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
