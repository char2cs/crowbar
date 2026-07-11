package realtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeWatcherProc struct {
	started chan struct{}
	stopped chan struct{}
	once    sync.Once
	// startErr, when set, makes Start fail immediately instead of blocking on ctx —
	// the real watcher's EMFILE-while-adding-watches failure.
	startErr error
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
	return ctx.Err()
}

func (f *fakeWatcherProc) Stop() {
	f.once.Do(func() { close(f.stopped) })
}

// fakeLingerTimer is the controllable timer injected into the WatcherManager so
// linger expiry is driven explicitly (fire / forceFire), never by wall-clock
// waits — satisfying the no-timing test rule.
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

// fire runs the expiry callback as a normal (un-raced) timer would: only if the
// timer was not already stopped.
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

// forceFire runs the expiry callback unconditionally, simulating the AfterFunc
// race where the callback is already scheduled and fires even though Stop
// returned false. It exercises the manager's generation guard.
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

// failingFactory builds procs whose Start fails immediately — the shape of a real
// watcher that dies inside Start (EMFILE while registering watches).
func failingFactory(
	procs chan<- *fakeWatcherProc,
	startErr error,
) (watcherFactory, *int32) {
	var calls int32
	factory := func(
		_ context.Context,
		_ string,
	) (watcherProc, error) {
		atomic.AddInt32(&calls, 1)
		p := newFakeWatcherProc()
		p.startErr = startErr
		procs <- p
		return p, nil
	}
	return factory, &calls
}

// newTestManager wires a WatcherManager over the counting factory and the
// controllable timer, returning the proc and timer channels plus the build
// counter so tests drive linger deterministically.
func newTestManager(
	t *testing.T,
) (*WatcherManager, chan *fakeWatcherProc, chan *fakeLingerTimer, *int32) {
	t.Helper()
	procs := make(chan *fakeWatcherProc, 16)
	timers := make(chan *fakeLingerTimer, 16)
	factory, calls := countingFactory(t, procs)
	m := newWatcherManager(context.Background(), factory, time.Second, recordingTimerFactory(timers))
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

func TestWatcherManager_LazyStart_LingersOnRelease(t *testing.T) {
	m, procs, timers, _ := newTestManager(t)

	m.Acquire("w1")
	p := <-procs
	<-p.started // first subscriber STARTS the watcher

	m.Release("w1")
	assertNotStopped(t, p, "watcher stopped immediately on release instead of lingering")

	ft := <-timers
	ft.fire() // linger elapses with no re-acquire
	<-p.stopped
}

func TestWatcherManager_RefcountNoDoubleStart(t *testing.T) {
	m, procs, timers, calls := newTestManager(t)

	m.Acquire("w1")
	p := <-procs
	<-p.started
	m.Acquire("w1") // second subscriber: no new watcher

	assert.EqualValues(t, 1, *calls)

	m.Release("w1") // still one subscriber left: alive, no linger armed
	assertNotStopped(t, p, "watcher stopped while a subscriber remained")

	m.Release("w1") // last subscriber leaves: linger
	ft := <-timers
	ft.fire()
	<-p.stopped
}

func TestWatcherManager_GitOnlyKeepsWatcherAlive(t *testing.T) {
	m, procs, timers, calls := newTestManager(t)

	m.Acquire("w1") // files subscriber starts it
	p := <-procs
	<-p.started
	m.Acquire("w1") // git subscriber shares the same refcount

	m.Release("w1") // files subscriber leaves; git keeps it alive
	assertNotStopped(t, p, "watcher stopped while git subscriber remained")

	m.Release("w1")
	ft := <-timers
	ft.fire()
	<-p.stopped
	assert.EqualValues(t, 1, *calls)
}

// A reconnect (Release then Acquire) that lands inside the linger window must
// reuse the live watcher: the factory is called exactly once no matter how many
// times the single subscriber flaps.
func TestWatcherManager_ReconnectWithinLingerReusesWatcher(t *testing.T) {
	m, procs, timers, calls := newTestManager(t)

	m.Acquire("w1")
	p := <-procs
	<-p.started

	for i := 0; i < 5; i++ {
		m.Release("w1") // arms a linger timer
		ft := <-timers
		m.Acquire("w1") // reconnect within the window: reuse, cancel the timer

		select {
		case <-procs:
			t.Fatal("watcher was rebuilt on reconnect instead of reused")
		default:
		}
		assert.False(t, ft.Stop(), "re-acquire must have cancelled the linger timer")
		assertNotStopped(t, p, "reused watcher must stay alive across reconnect")
	}

	assert.EqualValues(t, 1, *calls, "reconnect churn must collapse to a single build")

	m.Release("w1")
	ft := <-timers
	ft.fire()
	<-p.stopped
}

// Once the linger window fully elapses, the watcher is stopped and a later
// subscribe builds a fresh one.
func TestWatcherManager_ExpiredLingerRebuildsFresh(t *testing.T) {
	m, procs, timers, calls := newTestManager(t)

	m.Acquire("w1")
	p1 := <-procs
	<-p1.started

	m.Release("w1")
	ft := <-timers
	ft.fire() // linger fully elapses
	<-p1.stopped

	m.Acquire("w1") // a later subscribe builds a fresh watcher
	p2 := <-procs
	<-p2.started
	assert.EqualValues(t, 2, *calls, "a post-expiry subscribe must build a fresh watcher")

	m.Release("w1")
	ft2 := <-timers
	ft2.fire()
	<-p2.stopped
}

// TestWatcherManager_SupersededLingerExpiryIsIgnored proves the Acquire/expire
// race is safe: if a linger timer's callback fires AFTER a reconnect superseded
// it, the generation guard makes the expiry a no-op and the reused watcher
// survives.
func TestWatcherManager_SupersededLingerExpiryIsIgnored(t *testing.T) {
	m, procs, timers, _ := newTestManager(t)

	m.Acquire("w1")
	p := <-procs
	<-p.started

	m.Release("w1") // arm linger (generation G)
	ft := <-timers
	m.Acquire("w1") // reconnect: reuse, bump generation, cancel timer

	ft.forceFire() // the stale timer fires late anyway (race)
	assertNotStopped(t, p, "a superseded linger expiry stopped a reused watcher")

	m.Release("w1")
	ft2 := <-timers
	ft2.fire()
	<-p.stopped
}

func TestWatcherManager_BlankWsIDIgnored(t *testing.T) {
	m, _, _, calls := newTestManager(t)

	m.Acquire("")
	m.Release("")
	assert.EqualValues(t, 0, *calls)
}

func TestWatcherManager_ReleaseUnknownIsNoop(t *testing.T) {
	m, _, _, _ := newTestManager(t)

	m.Release("ghost") // must not panic
}

func TestWatcherManager_FactoryErrorDoesNotRegister(t *testing.T) {
	factory := func(
		_ context.Context,
		_ string,
	) (watcherProc, error) {
		return nil, errors.New("boom")
	}
	timers := make(chan *fakeLingerTimer, 1)
	m := newWatcherManager(context.Background(), factory, time.Second, recordingTimerFactory(timers))

	m.Acquire("w1")
	require.NotPanics(t, func() { m.Release("w1") })
}

// A watcher whose Start fails (EMFILE while adding watches) is dead: it emits no
// file-change and no git-status event, ever. Leaving its handle in the map would
// make every later Acquire bump the refcount of that corpse, silently blinding the
// workspace for the life of the subscription. The failed start must retire the
// handle so the next subscriber builds a fresh watcher.
func TestWatcherManager_FailedStartRetiresHandleAndRebuilds(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 4)
	timers := make(chan *fakeLingerTimer, 4)
	factory, calls := failingFactory(procs, errors.New("too many open files"))
	m := newWatcherManager(context.Background(), factory, time.Second, recordingTimerFactory(timers))

	m.Acquire("w1")
	p1 := <-procs
	<-p1.started
	<-p1.stopped // retirement stops the dead watcher: the signal that it left the map

	m.mu.Lock()
	_, present := m.handles["w1"]
	m.mu.Unlock()
	assert.False(t, present, "a watcher whose Start failed must not stay registered")

	m.Acquire("w1")
	p2 := <-procs
	<-p2.started
	assert.NotSame(t, p1, p2, "the retried Acquire must build a fresh watcher")
	assert.EqualValues(t, 2, atomic.LoadInt32(calls), "a failed start must be retried by the next Acquire")
}

// The retirement path must not clobber a handle it does not own: a proc whose Start
// fails LATE, after its handle already expired and a fresh Acquire rebuilt the
// watcher, would otherwise tear down the live replacement.
func TestWatcherManager_RetireIgnoresSupersededProc(t *testing.T) {
	m, procs, timers, calls := newTestManager(t)

	m.Acquire("w1")
	stale := <-procs
	<-stale.started

	m.Release("w1")
	ft := <-timers
	ft.fire() // linger elapses: the handle is torn down
	<-stale.stopped

	m.Acquire("w1") // a fresh watcher takes the slot
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

func TestWatcherManager_StopAllStopsLiveWatchers(t *testing.T) {
	m, procs, _, _ := newTestManager(t)

	m.Acquire("w1")
	p1 := <-procs
	<-p1.started
	m.Acquire("w2")
	p2 := <-procs
	<-p2.started

	m.StopAll()
	<-p1.stopped
	<-p2.stopped

	m.mu.Lock()
	remaining := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, remaining)
}

// StopAll must tear down a watcher that is currently lingering (post-release,
// pre-expiry) and cancel its pending linger timer.
func TestWatcherManager_StopAllStopsLingeringWatcher(t *testing.T) {
	m, procs, timers, _ := newTestManager(t)

	m.Acquire("w1")
	p := <-procs
	<-p.started
	m.Release("w1") // lingering
	ft := <-timers

	m.StopAll()
	<-p.stopped // StopAll stopped the lingering watcher
	assert.False(t, ft.Stop(), "StopAll must cancel the pending linger timer")
}

func TestWatcherManager_StopAllIdempotentAndEmptyNoop(t *testing.T) {
	m, procs, _, _ := newTestManager(t)

	m.Acquire("w1")
	p := <-procs
	<-p.started

	m.StopAll()
	<-p.stopped
	require.NotPanics(t, m.StopAll) // second call is safe
}

func TestWatcherManager_StopAllOnEmptyIsNoop(t *testing.T) {
	m, _, _, _ := newTestManager(t)

	require.NotPanics(t, m.StopAll) // no live watcher: no-op
}

func TestWatcherManager_ReleaseAfterStopAllIsNoop(t *testing.T) {
	m, procs, _, _ := newTestManager(t)

	m.Acquire("w1")
	p := <-procs
	<-p.started

	m.StopAll()
	<-p.stopped
	require.NotPanics(t, func() { m.Release("w1") })
}

func TestWatcherManager_AcquireAfterStopAllIsNoop(t *testing.T) {
	m, procs, _, calls := newTestManager(t)

	m.StopAll()
	m.Acquire("w1") // late subscribe after shutdown: no handle, nothing started

	assert.EqualValues(t, 0, *calls)
	m.mu.Lock()
	remaining := len(m.handles)
	m.mu.Unlock()
	assert.Equal(t, 0, remaining)

	select {
	case <-procs:
		t.Fatal("a watcher was started after StopAll")
	default:
	}
}
