package stream_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

var errClosed = errors.New("fake conn: closed")

// fakeConn is a client over channels. With throttle set, each write waits for
// a token on allow, which is how a test holds the writer still.
type fakeConn struct {
	in       chan stream.Request
	out      chan stream.Frame
	throttle bool
	allow    chan struct{}

	closeOnce sync.Once
	closed    chan struct{}
	mu        sync.Mutex
	code      int
}

func newFakeConn() *fakeConn {
	return &fakeConn{
		in:     make(chan stream.Request),
		out:    make(chan stream.Frame, 4*stream.QueueSize),
		allow:  make(chan struct{}, 4*stream.QueueSize),
		closed: make(chan struct{}),
	}
}

func (f *fakeConn) ReadJSON(v any) error {
	select {
	case req := <-f.in:
		out, ok := v.(*stream.Request)
		if !ok {
			return errors.New("fake conn: reads only requests")
		}
		*out = req
		return nil
	case <-f.closed:
		return errClosed
	}
}

func (f *fakeConn) WriteJSON(v any) error {
	if f.throttle {
		select {
		case <-f.allow:
		case <-f.closed:
			return errClosed
		}
	}
	frame, ok := v.(stream.Frame)
	if !ok {
		return errors.New("fake conn: writes only frames")
	}
	select {
	case f.out <- frame:
		return nil
	case <-f.closed:
		return errClosed
	}
}

func (f *fakeConn) CloseWith(code int, _ string) error {
	f.closeOnce.Do(func() {
		f.mu.Lock()
		f.code = code
		f.mu.Unlock()
		close(f.closed)
	})
	return nil
}

func (f *fakeConn) closeCode() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.code
}

// waitBound fails a test whose awaited frame never comes instead of hanging it.
const waitBound = 10 * time.Second

// ended waits for Serve to return.
func ended(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(waitBound):
		require.FailNow(t, "Serve did not return")
	}
}

func (f *fakeConn) send(t *testing.T, op, scope string) {
	t.Helper()
	select {
	case f.in <- stream.Request{Op: op, Scope: scope}:
	case <-f.closed:
		require.FailNow(t, "connection closed before "+op+" "+scope)
	}
}

func (f *fakeConn) next(t *testing.T) stream.Frame {
	t.Helper()
	select {
	case frame := <-f.out:
		return frame
	case <-time.After(waitBound):
		require.FailNow(t, "no frame arrived")
		return stream.Frame{}
	}
}

// until reads frames up to and including the first that matches.
func (f *fakeConn) until(t *testing.T, match func(stream.Frame) bool) []stream.Frame {
	t.Helper()
	var seen []stream.Frame
	for {
		frame := f.next(t)
		seen = append(seen, frame)
		if match(frame) {
			return seen
		}
	}
}

func syncedOn(scope string) func(stream.Frame) bool {
	return func(f stream.Frame) bool { return f.T == stream.FrameSynced && f.Scope == scope }
}

// sub holds scope and returns its snapshot's frames, synced excluded.
func (f *fakeConn) sub(t *testing.T, scope string) []stream.Frame {
	t.Helper()
	f.send(t, stream.OpSub, scope)
	seen := f.until(t, syncedOn(scope))
	return seen[:len(seen)-1]
}

// served runs hub.Serve on conn, consumes its ready frame and returns a
// channel closed once Serve returns.
func served(t *testing.T, hub *appstream.Hub, conn *fakeConn) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = hub.Serve(t.Context(), conn)
	}()
	if conn.throttle {
		conn.allow <- struct{}{}
	}
	require.Equal(t, stream.FrameReady, conn.next(t).T)
	t.Cleanup(func() {
		_ = conn.CloseWith(1000, "")
		<-done
	})
	return done
}

// entities is a fake entity source: a map snapshotted on attach.
type entities struct {
	mu   sync.Mutex
	rows map[string]string
}

func newEntities(rows map[string]string) *entities { return &entities{rows: rows} }

func (e *entities) set(id, value string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rows[id] = value
}

func (e *entities) Attach(_ context.Context, _ string, _ int64, emit func(stream.Frame)) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, value := range e.rows {
		data, _ := json.Marshal(value)
		emit(stream.Frame{T: stream.FrameEntity, Kind: string(stream.KindOperation), ID: id, Data: data})
	}
	return nil
}

func encodeJSON(_ stream.EntityKind, v any) (json.RawMessage, error) { return json.Marshal(v) }

// recorder is a fake resource counting its starts and stops per key.
type recorder struct {
	mu     sync.Mutex
	starts map[string]int
	stops  map[string]int
}

func newRecorder() *recorder { return &recorder{starts: map[string]int{}, stops: map[string]int{}} }

func (r *recorder) Start(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts[key]++
}

func (r *recorder) Stop(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stops[key]++
}

func (r *recorder) counts(key string) (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.starts[key], r.stops[key]
}

func newHub(sources map[stream.ScopeKind]appstream.Source, resources map[stream.ScopeKind][]appstream.Resource) *appstream.Hub {
	return appstream.New("epoch-1", sources, resources, encodeJSON)
}
