package resources

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeWatcherProc struct {
	started chan struct{}
	stopped chan struct{}
	once    sync.Once
	// startErr, when set, makes Start fail at once (EMFILE while adding watches).
	startErr error
	// startTail and stopTail, when set, keep Start (once its ctx ends) or Stop
	// running until closed: work still in flight when Close runs.
	startTail, stopTail chan struct{}
}

func newFakeWatcherProc() *fakeWatcherProc {
	return &fakeWatcherProc{
		started: make(chan struct{}, 1),
		stopped: make(chan struct{}),
	}
}

func (f *fakeWatcherProc) Start(
	ctx context.Context,
) error {
	f.started <- struct{}{}
	if f.startErr != nil {
		return f.startErr
	}
	<-ctx.Done()
	if f.startTail != nil {
		<-f.startTail
	}
	return ctx.Err()
}

func (f *fakeWatcherProc) Stop() {
	f.once.Do(func() { close(f.stopped) })
	if f.stopTail != nil {
		<-f.stopTail
	}
}

// fakeLingerTimer drives linger expiry explicitly (fire / forceFire), never by
// wall-clock waits.
type fakeLingerTimer struct {
	mu      sync.Mutex
	fn      func()
	stopped bool
	fired   bool
}

func (t *fakeLingerTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped || t.fired {
		return false
	}
	t.stopped = true
	return true
}

// fire runs the expiry callback only if the timer was not already stopped.
func (t *fakeLingerTimer) fire() {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.fired = true
	fn := t.fn
	t.mu.Unlock()
	fn()
}

// forceFire runs the expiry callback unconditionally: the AfterFunc race where a
// scheduled callback fires even though Stop returned false.
func (t *fakeLingerTimer) forceFire() {
	t.mu.Lock()
	t.fired = true
	fn := t.fn
	t.mu.Unlock()
	fn()
}

func recordingTimerFactory(
	timers chan<- *fakeLingerTimer,
) timerFactory {
	return func(
		_ time.Duration,
		fn func(),
	) lingerTimer {
		ft := &fakeLingerTimer{fn: fn}
		timers <- ft
		return ft
	}
}

func countingFactory(
	t *testing.T,
	procs chan<- *fakeWatcherProc,
) (watcherFactory, *int32) {
	t.Helper()
	var calls int32
	var mu sync.Mutex
	factory := func(
		_ context.Context,
		_ string,
	) (watcherProc, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		p := newFakeWatcherProc()
		procs <- p
		return p, nil
	}
	return factory, &calls
}

// shapedFactory builds procs that shape adjusts before they start: one whose
// Start fails, or one whose Start or Stop is still running at Close.
func shapedFactory(
	procs chan<- *fakeWatcherProc,
	shape func(*fakeWatcherProc),
) (watcherFactory, *int32) {
	var calls int32
	factory := func(
		_ context.Context,
		_ string,
	) (watcherProc, error) {
		atomic.AddInt32(&calls, 1)
		p := newFakeWatcherProc()
		shape(p)
		procs <- p
		return p, nil
	}
	return factory, &calls
}

// newTestManager wires a WatcherManager over the counting factory and the
// controllable timer, so tests drive linger deterministically.
func newTestManager(
	t *testing.T,
) (*Watcher, chan *fakeWatcherProc, chan *fakeLingerTimer, *int32) {
	t.Helper()
	procs := make(chan *fakeWatcherProc, 16)
	timers := make(chan *fakeLingerTimer, 16)
	factory, calls := countingFactory(t, procs)
	m := newWatcher(context.Background(), factory, time.Second, recordingTimerFactory(timers))
	return m, procs, timers, calls
}

// assertNotStopped fails if the watcher has already been stopped.
func assertNotStopped(
	t *testing.T,
	p *fakeWatcherProc,
	msg string,
) {
	t.Helper()
	select {
	case <-p.stopped:
		t.Fatal(msg)
	default:
	}
}

func TestWatcher_LazyStart_LingersOnStop(t *testing.T) {
	m, procs, timers, _ := newTestManager(t)

	require.NoError(t, m.Start("w1"))
	p := <-procs
	<-p.started // first holder STARTS the watcher

	m.Stop("w1")
	assertNotStopped(t, p, "watcher stopped immediately on release instead of lingering")

	ft := <-timers
	ft.fire() // linger elapses with no re-acquire
	<-p.stopped
}

func TestWatcher_ReconnectWithinLingerReusesWatcher(t *testing.T) {
	m, procs, timers, calls := newTestManager(t)

	require.NoError(t, m.Start("w1"))
	p := <-procs
	<-p.started

	for i := 0; i < 5; i++ {
		m.Stop("w1") // arms a linger timer
		ft := <-timers
		require.NoError(t, m.Start("w1")) // reconnect within the window: reuse, cancel the timer

		select {
		case <-procs:
			t.Fatal("watcher was rebuilt on reconnect instead of reused")
		default:
		}
		assert.False(t, ft.Stop(), "re-acquire must have cancelled the linger timer")
		assertNotStopped(t, p, "reused watcher must stay alive across reconnect")
	}

	assert.EqualValues(t, 1, *calls, "reconnect churn must collapse to a single build")

	m.Stop("w1")
	ft := <-timers
	ft.fire()
	<-p.stopped
}

// Once the linger elapses the watcher stops, and a later hold builds anew.
func TestWatcher_ExpiredLingerRebuildsFresh(t *testing.T) {
	m, procs, timers, calls := newTestManager(t)

	require.NoError(t, m.Start("w1"))
	p1 := <-procs
	<-p1.started

	m.Stop("w1")
	ft := <-timers
	ft.fire() // linger fully elapses
	<-p1.stopped

	require.NoError(t, m.Start("w1")) // a later hold builds a fresh watcher
	p2 := <-procs
	<-p2.started
	assert.EqualValues(t, 2, *calls, "a post-expiry hold must build a fresh watcher")

	m.Stop("w1")
	ft2 := <-timers
	ft2.fire()
	<-p2.stopped
}

// TestWatcher_SupersededLingerExpiryIsIgnored: a linger callback firing
// after a reconnect superseded it is a no-op, so the reused watcher survives.
func TestWatcher_SupersededLingerExpiryIsIgnored(t *testing.T) {
	m, procs, timers, _ := newTestManager(t)

	require.NoError(t, m.Start("w1"))
	p := <-procs
	<-p.started

	m.Stop("w1") // arm linger (generation G)
	ft := <-timers
	require.NoError(t, m.Start("w1")) // reconnect: reuse, bump generation, cancel timer

	ft.forceFire() // the stale timer fires late anyway (race)
	assertNotStopped(t, p, "a superseded linger expiry stopped a reused watcher")

	m.Stop("w1")
	ft2 := <-timers
	ft2.fire()
	<-p.stopped
}

func TestWatcher_StopUnknownIsNoop(t *testing.T) {
	m, _, _, _ := newTestManager(t)

	m.Stop("ghost") // must not panic
}

// A workspace that cannot be watched is refused, so the client hears why.
func TestWatcher_AFactoryErrorRefusesTheStartAndRegistersNothing(t *testing.T) {
	boom := errors.New("boom")
	factory := func(
		_ context.Context,
		_ string,
	) (watcherProc, error) {
		return nil, boom
	}
	timers := make(chan *fakeLingerTimer, 1)
	m := newWatcher(context.Background(), factory, time.Second, recordingTimerFactory(timers))

	require.ErrorIs(t, m.Start("w1"), boom)
	m.Stop("w1")
	assert.Empty(t, timers, "nothing registered, so nothing lingers")
}

// A watcher whose Start fails is dead: left in the map, every later Start would
// refcount the corpse and blind the workspace. The failed start must retire the
// handle so the next holder builds a fresh watcher.
func TestWatcher_FailedStartRetiresHandleAndRebuilds(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 4)
	timers := make(chan *fakeLingerTimer, 4)
	tooManyFiles := errors.New("too many open files")
	factory, calls := shapedFactory(procs, func(p *fakeWatcherProc) { p.startErr = tooManyFiles })
	m := newWatcher(context.Background(), factory, time.Second, recordingTimerFactory(timers))

	require.NoError(t, m.Start("w1"))
	p1 := <-procs
	<-p1.started
	<-p1.stopped // retirement stops the dead watcher: the signal that it left the map

	m.mu.Lock()
	_, present := m.handles["w1"]
	m.mu.Unlock()
	assert.False(t, present, "a watcher whose Start failed must not stay registered")

	require.NoError(t, m.Start("w1"))
	p2 := <-procs
	<-p2.started
	assert.NotSame(t, p1, p2, "the retried Start must build a fresh watcher")
	assert.EqualValues(t, 2, atomic.LoadInt32(calls), "a failed start must be retried by the next Start")
}

// The retirement path must not clobber a handle it does not own: a proc whose Start
// fails LATE, after its handle already expired and a fresh Start rebuilt the
// watcher, would otherwise tear down the live replacement.
func TestWatcher_RetireIgnoresSupersededProc(t *testing.T) {
	m, procs, timers, calls := newTestManager(t)

	require.NoError(t, m.Start("w1"))
	stale := <-procs
	<-stale.started

	m.Stop("w1")
	ft := <-timers
	ft.fire() // linger elapses: the handle is torn down
	<-stale.stopped

	require.NoError(t, m.Start("w1")) // a fresh watcher takes the slot
	live := <-procs
	<-live.started

	m.retire("w1", stale) // the superseded proc reports its failure late

	m.mu.Lock()
	h, present := m.handles["w1"]
	m.mu.Unlock()
	require.True(t, present, "a superseded proc's failure must not retire the current handle")
	assert.Same(t, live, h.proc)
	assertNotStopped(t, live, "a superseded proc's failure stopped the live watcher")
	assert.EqualValues(t, 2, *calls)
}

func TestWatcher_CloseStopsLiveWatchers(t *testing.T) {
	m, procs, _, _ := newTestManager(t)

	require.NoError(t, m.Start("w1"))
	p1 := <-procs
	<-p1.started
	require.NoError(t, m.Start("w2"))
	p2 := <-procs
	<-p2.started

	m.Close()
	<-p1.stopped
	<-p2.stopped

	m.mu.Lock()
	remaining := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, remaining)
}

// Close tears down a lingering watcher and cancels its pending linger timer.
func TestWatcher_CloseStopsLingeringWatcher(t *testing.T) {
	m, procs, timers, _ := newTestManager(t)

	require.NoError(t, m.Start("w1"))
	p := <-procs
	<-p.started
	m.Stop("w1") // lingering
	ft := <-timers

	m.Close()
	<-p.stopped // Close stopped the lingering watcher
	assert.False(t, ft.Stop(), "Close must cancel the pending linger timer")
}

func TestWatcher_CloseIdempotentAndEmptyNoop(t *testing.T) {
	m, procs, _, _ := newTestManager(t)

	require.NoError(t, m.Start("w1"))
	p := <-procs
	<-p.started

	m.Close()
	<-p.stopped
	require.NotPanics(t, m.Close) // second call is safe
}

func TestWatcher_CloseOnEmptyIsNoop(t *testing.T) {
	m, _, _, _ := newTestManager(t)

	require.NotPanics(t, m.Close) // no live watcher: no-op
}

func TestWatcher_StopAfterCloseIsNoop(t *testing.T) {
	m, procs, _, _ := newTestManager(t)

	require.NoError(t, m.Start("w1"))
	p := <-procs
	<-p.started

	m.Close()
	<-p.stopped
	require.NotPanics(t, func() { m.Stop("w1") })
}

func TestWatcher_StartAfterCloseIsRefused(t *testing.T) {
	m, procs, _, calls := newTestManager(t)

	m.Close()
	require.ErrorIs(t, m.Start("w1"), errClosed) // a late hold after shutdown starts nothing

	assert.EqualValues(t, 0, *calls)
	m.mu.Lock()
	remaining := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, remaining)

	select {
	case <-procs:
		t.Fatal("a watcher was started after Close")
	default:
	}
}

func TestWatcher_CloseJoinsAStartInFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tail, procs := make(chan struct{}), make(chan *fakeWatcherProc, 1)
		factory, _ := shapedFactory(procs, func(p *fakeWatcherProc) { p.startTail = tail })
		m := newWatcher(context.Background(), factory, time.Second, nil)
		require.NoError(t, m.Start("w1"))
		<-(<-procs).started

		assert.False(t, stopReturnedEarly(m.Close, tail), "Close returned while a Start was still running")
	})
}

func TestWatcher_CloseJoinsAnExpiryInFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tail, procs, timers := make(chan struct{}), make(chan *fakeWatcherProc, 1), make(chan *fakeLingerTimer, 1)
		factory, _ := shapedFactory(procs, func(p *fakeWatcherProc) { p.stopTail = tail })
		m := newWatcher(context.Background(), factory, time.Second, recordingTimerFactory(timers))
		require.NoError(t, m.Start("w1"))
		m.Stop("w1")
		go (<-timers).fire()
		synctest.Wait()

		assert.False(t, stopReturnedEarly(m.Close, tail), "Close returned while an expiry was still stopping its watcher")
	})
}
