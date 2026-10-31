package resources

import (
	"context"
	"encoding/json"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// pipeConn is a stream client: requests go in, frames come out.
type pipeConn struct {
	in  chan stream.Request
	out chan stream.Frame
}

func (p pipeConn) ReadJSON(v any) error {
	req, ok := <-p.in
	if !ok {
		return io.EOF
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func (p pipeConn) WriteJSON(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var f stream.Frame
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	p.out <- f
	return nil
}

func (pipeConn) CloseWith(int, string) error { return nil }

func (p pipeConn) hold(t *testing.T, scope string) {
	t.Helper()
	p.in <- stream.Request{Op: stream.OpSub, Scope: scope}
	for f := range p.out {
		if f.T == stream.FrameSynced && f.Scope == scope {
			return
		}
	}
}

func TestRefcount_TheWatcherIsSharedByGitAndFiles(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 2)
	timers := make(chan *fakeLingerTimer, 2)
	factory, calls := countingFactory(t, procs)
	w := newWatcher(context.Background(), factory, time.Second, recordingTimerFactory(timers))
	t.Cleanup(w.Close)
	hub := appstream.New("e1",
		map[stream.ScopeKind]appstream.Source{stream.KindGit: appstream.Quiet{}, stream.KindFiles: appstream.Quiet{}},
		map[stream.ScopeKind][]appstream.Resource{stream.KindGit: {w}, stream.KindFiles: {w}},
		nil,
	)
	conn := pipeConn{in: make(chan stream.Request), out: make(chan stream.Frame, 16)}
	done := make(chan error, 1)
	go func() { done <- hub.Serve(context.Background(), conn) }()

	conn.hold(t, "git:w1")
	conn.hold(t, "files:w1")
	proc := <-procs
	<-proc.started
	assert.Equal(t, int32(1), atomic.LoadInt32(calls), "git: and files: share one watcher")

	close(conn.in)
	<-done
	ft := <-timers
	assert.Empty(t, timers, "both holds end in one stop")
	select {
	case <-proc.stopped:
		t.Fatal("the watcher stopped before its linger elapsed")
	default:
	}
	ft.fire()
	<-proc.stopped
	require.Equal(t, int32(1), atomic.LoadInt32(calls))
}
