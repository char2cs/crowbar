package resources

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
)

// hostWaitBound fails a test whose signal never comes instead of stalling it.
const hostWaitBound = 10 * time.Second

// fakeFileWatch records its lifecycle and exposes the emit it was built with.
type fakeFileWatch struct {
	path string
	emit func(domain.FileChangeType)
	// startGate and stopGate, when set, hold Start or Stop until closed.
	startGate, stopGate chan struct{}
	startErr            error
	started             chan struct{}
	stops               atomic.Int32
}

func (f *fakeFileWatch) Start(context.Context) error {
	if f.startGate != nil {
		<-f.startGate
	}
	close(f.started)
	return f.startErr
}

func (f *fakeFileWatch) Stop() {
	f.stops.Add(1)
	if f.stopGate != nil {
		<-f.stopGate
	}
}

// hostWatchRig is the resource over fake watches and a recording broadcast.
type hostWatchRig struct {
	m      *HostWatch
	built  chan *fakeFileWatch
	events chan domain.FileChangeEvent
	// prepare, when set, configures each watch before the manager starts it.
	prepare func(*fakeFileWatch)
	// during, when set, runs inside each broadcast before it is recorded.
	during func()
}

func newHostWatchRig(
	t *testing.T,
) *hostWatchRig {
	t.Helper()
	r := &hostWatchRig{built: make(chan *fakeFileWatch, 16), events: make(chan domain.FileChangeEvent, 16)}
	factory := func(path string, emit func(domain.FileChangeType)) watcherProc {
		f := &fakeFileWatch{path: path, emit: emit, started: make(chan struct{})}
		if r.prepare != nil {
			r.prepare(f)
		}
		r.built <- f
		return f
	}
	r.m = newHostWatch(t.Context(), factory, func(e domain.FileChangeEvent) {
		if r.during != nil {
			r.during()
		}
		r.events <- e
	})
	t.Cleanup(r.m.Close)
	return r
}

func (r *hostWatchRig) next(
	t *testing.T,
) *fakeFileWatch {
	t.Helper()
	select {
	case f := <-r.built:
		return f
	case <-time.After(hostWaitBound):
		require.FailNow(t, "no watch built")
		return nil
	}
}

func (r *hostWatchRig) assertNoneBuilt(
	t *testing.T,
) {
	t.Helper()
	select {
	case f := <-r.built:
		assert.Fail(t, "unexpected watch", f.path)
	default:
	}
}

// within runs fn and answers its error, or that fn is still blocked after the
// bound: a lock held where it must not be fails the test, never hangs it.
func within(
	what string,
	fn func() error,
) error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(hostWaitBound):
		return fmt.Errorf("%s is still blocked after %s", what, hostWaitBound)
	}
}

func TestHostWatch_ArmsBeforeStartReturns(t *testing.T) {
	r := newHostWatchRig(t)

	require.NoError(t, r.m.Start("/d/a.md"))

	select {
	case <-r.next(t).started:
	default:
		assert.Fail(t, "Start returned before the watch was armed")
	}
}

func TestHostWatch_StopStopsTheWatch(t *testing.T) {
	r := newHostWatchRig(t)
	require.NoError(t, r.m.Start("/d/a.md"))
	first := r.next(t)

	r.m.Stop("/d/a.md")
	assert.Equal(t, int32(1), first.stops.Load())
	r.m.Stop("/d/a.md")
	assert.Equal(t, int32(1), first.stops.Load(), "a second stop finds nothing")

	require.NoError(t, r.m.Start("/d/a.md"))
	assert.NotSame(t, first, r.next(t), "a restart arms a fresh watch")
}

func TestHostWatch_BroadcastsTheHeldPath(t *testing.T) {
	r := newHostWatchRig(t)
	require.NoError(t, r.m.Start("/d/a.md"))

	r.next(t).emit(domain.FileChangeModified)

	want := domain.FileChangeEvent{Type: domain.FileChangeModified, Authority: domain.DocumentAuthorityHost, Path: "/d/a.md"}
	assert.Equal(t, want, <-r.events)
}

func TestHostWatch_AChangeWhileItsWatchStopsIsDropped(t *testing.T) {
	r := newHostWatchRig(t)
	stopGate := make(chan struct{})
	r.prepare = func(f *fakeFileWatch) { f.stopGate = stopGate }
	require.NoError(t, r.m.Start("/d/a.md"))
	old := r.next(t)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		r.m.Stop("/d/a.md")
	}()
	require.Eventually(t, func() bool { return old.stops.Load() == 1 }, hostWaitBound, time.Millisecond)

	old.emit(domain.FileChangeModified)
	close(stopGate)
	<-stopped
	r.prepare = nil
	require.NoError(t, r.m.Start("/d/a.md"))
	r.next(t).emit(domain.FileChangeCreated)

	assert.Equal(t, domain.FileChangeCreated, (<-r.events).Type, "only the current watch broadcasts")
}

func TestHostWatch_CloseStopsEveryWatchAndRefusesLaterStarts(t *testing.T) {
	r := newHostWatchRig(t)
	require.NoError(t, r.m.Start("/d/a.md"))
	require.NoError(t, r.m.Start("/d/b.md"))
	a, b := r.next(t), r.next(t)

	r.m.Close()

	assert.Equal(t, int32(1), a.stops.Load())
	assert.Equal(t, int32(1), b.stops.Load())
	require.ErrorIs(t, r.m.Start("/d/c.md"), errClosed)
	r.assertNoneBuilt(t)
}

func TestHostWatch_AnArmFailureRecordsNothing(t *testing.T) {
	r := newHostWatchRig(t)
	boom := errors.New("the mount went away")
	r.prepare = func(f *fakeFileWatch) { f.startErr = boom }

	require.ErrorIs(t, r.m.Start("/d/a.md"), boom)
	failed := r.next(t)

	assert.Zero(t, failed.stops.Load(), "a watch that never armed has nothing to stop")
	r.prepare = nil
	require.NoError(t, r.m.Start("/d/a.md"))
	assert.NotSame(t, failed, r.next(t), "the failed watch was not recorded")
}

// The client tells the OS's watch limit apart from a daemon that cannot watch the file.
func TestHostWatch_TheWatchLimitIsRefusedWithItsOwnCode(t *testing.T) {
	r := newHostWatchRig(t)
	r.prepare = func(f *fakeFileWatch) { f.startErr = fmt.Errorf("arm: %w", enginefs.ErrWatchLimit) }

	err := r.m.Start("/d/a.md")

	require.ErrorIs(t, err, enginefs.ErrWatchLimit)
	assert.Equal(t, problem.WatchLimit, problem.CodeOf(err))
	r.prepare = func(f *fakeFileWatch) { f.startErr = errors.New("no such mount") }
	assert.NotEqual(t, problem.WatchLimit, problem.CodeOf(r.m.Start("/d/b.md")))
}

func TestHostWatch_ASlowArmHoldsBackOnlyItsOwnPath(t *testing.T) {
	r := newHostWatchRig(t)
	gate := make(chan struct{})
	r.prepare = func(f *fakeFileWatch) {
		if f.path == "/d/slow.md" {
			f.startGate = gate
		}
	}
	slow := make(chan error, 1)
	go func() { slow <- r.m.Start("/d/slow.md") }()
	r.next(t)

	err := within("another path's Start", func() error { return r.m.Start("/d/a.md") })

	close(gate)
	require.NoError(t, <-slow)
	require.NoError(t, err)
}

func TestHostWatch_ABroadcastHoldsBackNoStartOrStop(t *testing.T) {
	r := newHostWatchRig(t)
	require.NoError(t, r.m.Start("/d/a.md"))
	a := r.next(t)
	gate, inside := make(chan struct{}), make(chan struct{})
	r.during = func() {
		close(inside)
		<-gate
	}
	go a.emit(domain.FileChangeModified)
	select {
	case <-inside:
	case <-time.After(hostWaitBound):
		require.FailNow(t, "the emit never broadcast")
	}

	acquired := within("a Start during a broadcast", func() error { return r.m.Start("/d/b.md") })
	released := within("a Stop during a broadcast", func() error { r.m.Stop("/d/b.md"); return nil })

	close(gate)
	require.NoError(t, acquired)
	require.NoError(t, released)
	assert.Equal(t, "/d/a.md", (<-r.events).Path)
}
