package realtime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
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

// hostWatchRig is a manager over fake watches and a recording broadcast.
type hostWatchRig struct {
	m      *HostWatchManager
	built  chan *fakeFileWatch
	events chan domain.FileChangeEvent
	// prepare, when set, configures each watch before the manager starts it.
	prepare func(*fakeFileWatch)
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
	r.m = newHostWatchManager(t.Context(), factory, func(e domain.FileChangeEvent) { r.events <- e })
	t.Cleanup(r.m.StopAll)
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

func holdersOf(
	m *HostWatchManager,
	path string,
) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if hw, ok := m.watches[path]; ok {
		return hw.holders
	}
	return 0
}

func TestHostWatchManager_OneWatchPerHeldPath(t *testing.T) {
	r := newHostWatchRig(t)

	require.NoError(t, r.m.Acquire("/d/a.md"))
	require.NoError(t, r.m.Acquire("/d/a.md"))
	require.NoError(t, r.m.Acquire("/d/b.md"))

	assert.Equal(t, "/d/a.md", r.next(t).path)
	assert.Equal(t, "/d/b.md", r.next(t).path)
	r.assertNoneBuilt(t)
}

func TestHostWatchManager_ArmsBeforeAcquireReturns(t *testing.T) {
	r := newHostWatchRig(t)

	require.NoError(t, r.m.Acquire("/d/a.md"))

	select {
	case <-r.next(t).started:
	default:
		assert.Fail(t, "Acquire returned before the watch was armed")
	}
}

func TestHostWatchManager_LastReleaseStopsTheWatch(t *testing.T) {
	r := newHostWatchRig(t)
	require.NoError(t, r.m.Acquire("/d/a.md"))
	require.NoError(t, r.m.Acquire("/d/a.md"))
	first := r.next(t)

	r.m.Release("/d/a.md")
	assert.Zero(t, first.stops.Load(), "a holder remains")
	r.m.Release("/d/a.md")
	assert.Equal(t, int32(1), first.stops.Load())

	require.NoError(t, r.m.Acquire("/d/a.md"))
	assert.NotSame(t, first, r.next(t), "a re-acquire arms a fresh watch")
}

func TestHostWatchManager_BroadcastsTheHeldPath(t *testing.T) {
	r := newHostWatchRig(t)
	require.NoError(t, r.m.Acquire("/d/a.md"))

	r.next(t).emit(domain.FileChangeModified)

	want := domain.FileChangeEvent{Type: domain.FileChangeModified, Authority: domain.DocumentAuthorityHost, Path: "/d/a.md"}
	assert.Equal(t, want, <-r.events)
}

func TestHostWatchManager_StaleWatcherNeverReachesItsSuccessor(t *testing.T) {
	r := newHostWatchRig(t)
	stopGate := make(chan struct{})
	r.prepare = func(f *fakeFileWatch) { f.stopGate = stopGate }
	require.NoError(t, r.m.Acquire("/d/a.md"))
	old := r.next(t)
	released := make(chan struct{})
	go func() {
		defer close(released)
		r.m.Release("/d/a.md")
	}()
	require.Eventually(t, func() bool { return old.stops.Load() == 1 }, hostWaitBound, time.Millisecond)

	require.NoError(t, r.m.Acquire("/d/a.md"))
	successor := r.next(t)
	old.emit(domain.FileChangeModified)
	successor.emit(domain.FileChangeCreated)

	assert.Equal(t, domain.FileChangeCreated, (<-r.events).Type, "only the current watch broadcasts")
	close(stopGate)
	<-released
}

func TestHostWatchManager_StopAllStopsEveryWatchAndRefusesLaterAcquires(t *testing.T) {
	r := newHostWatchRig(t)
	require.NoError(t, r.m.Acquire("/d/a.md"))
	require.NoError(t, r.m.Acquire("/d/b.md"))
	a, b := r.next(t), r.next(t)

	r.m.StopAll()

	assert.Equal(t, int32(1), a.stops.Load())
	assert.Equal(t, int32(1), b.stops.Load())
	require.ErrorIs(t, r.m.Acquire("/d/c.md"), errHostWatchesClosed)
	r.assertNoneBuilt(t)
}

func TestHostWatchManager_AnArmFailureRefusesEveryWaiterAndRecordsNothing(t *testing.T) {
	r := newHostWatchRig(t)
	gate := make(chan struct{})
	boom := errors.New("no inotify watches left")
	r.prepare = func(f *fakeFileWatch) { f.startGate, f.startErr = gate, boom }
	firstErr := make(chan error, 1)
	go func() { firstErr <- r.m.Acquire("/d/a.md") }()
	failed := r.next(t)
	waiterErr := make(chan error, 1)
	go func() { waiterErr <- r.m.Acquire("/d/a.md") }()
	require.Eventually(t, func() bool { return holdersOf(r.m, "/d/a.md") == 2 }, hostWaitBound, time.Millisecond)

	close(gate)

	require.ErrorIs(t, <-firstErr, boom)
	require.ErrorIs(t, <-waiterErr, boom)
	assert.Zero(t, failed.stops.Load(), "a watch that never armed has nothing to stop")
	r.prepare = nil
	require.NoError(t, r.m.Acquire("/d/a.md"))
	assert.NotSame(t, failed, r.next(t), "the failed watch was not recorded")
}

func TestHostWatchManager_ASlowArmHoldsBackOnlyItsOwnPath(t *testing.T) {
	r := newHostWatchRig(t)
	gate := make(chan struct{})
	r.prepare = func(f *fakeFileWatch) {
		if f.path == "/d/slow.md" {
			f.startGate = gate
		}
	}
	slow := make(chan error, 1)
	go func() { slow <- r.m.Acquire("/d/slow.md") }()
	r.next(t)

	require.NoError(t, r.m.Acquire("/d/a.md"), "another path arms while the slow one is still arming")
	r.m.Release("/d/a.md")

	close(gate)
	require.NoError(t, <-slow)
}
