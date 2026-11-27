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

type fakeConn struct {
	in       chan stream.Request
	bad      chan error
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
		bad:    make(chan error),
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
	case err := <-f.bad:
		return err
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

const waitBound = 10 * time.Second

func ended(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(waitBound):
		require.FailNow(t, "it never ended")
	}
}

func (f *fakeConn) send(t *testing.T, op, scope string) {
	t.Helper()
	f.sendRequest(t, stream.Request{Op: op, Scope: scope})
}

func (f *fakeConn) sendRequest(t *testing.T, req stream.Request) {
	t.Helper()
	op, scope := req.Op, req.Scope
	select {
	case f.in <- req:
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

func (f *fakeConn) sub(t *testing.T, scope string) []stream.Frame {
	t.Helper()
	f.send(t, stream.OpSub, scope)
	seen := f.until(t, syncedOn(scope))
	return seen[:len(seen)-1]
}

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

type recorder struct {
	mu     sync.Mutex
	starts map[string]int
	stops  map[string]int
	fail   map[string]error
	log    []string

	gated   bool
	entered chan string
	gate    chan struct{}

	stopGate chan struct{}
	stopping map[string]bool
	overlaps int
}

func newRecorder() *recorder {
	return &recorder{
		starts: map[string]int{}, stops: map[string]int{}, fail: map[string]error{},
		entered: make(chan string, 16), gate: make(chan struct{}), stopping: map[string]bool{},
	}
}

func (r *recorder) wait(what string) {
	if !r.gated {
		return
	}
	r.entered <- what
	<-r.gate
}

func (r *recorder) Start(key string) error {
	r.mu.Lock()
	if r.stopping[key] {
		r.overlaps++
	}
	r.mu.Unlock()
	r.wait("start " + key)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starts[key]++
	r.log = append(r.log, "start "+key)
	return r.fail[key]
}

func (r *recorder) Stop(key string) {
	r.wait("stop " + key)
	if r.stopGate != nil {
		r.mu.Lock()
		r.stopping[key] = true
		r.mu.Unlock()
		r.entered <- "stop " + key
		<-r.stopGate
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stops[key]++
	r.log = append(r.log, "stop "+key)
	r.stopping[key] = false
}

func (r *recorder) history() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.log...)
}

func (r *recorder) enteredAs(t *testing.T, what string) {
	t.Helper()
	select {
	case got := <-r.entered:
		require.Equal(t, what, got)
	case <-time.After(waitBound):
		require.FailNow(t, "never entered "+what)
	}
}

func (r *recorder) counts(key string) (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.starts[key], r.stops[key]
}

func newHub(sources map[stream.ScopeKind]appstream.Source, resources map[stream.ScopeKind][]appstream.Resource) *appstream.Hub {
	return appstream.New("epoch-1", sources, resources, encodeJSON)
}
