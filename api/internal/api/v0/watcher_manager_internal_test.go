package v0

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeWatcherProc struct {
	started chan struct{}
	stopped chan struct{}
	once    sync.Once
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
	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeWatcherProc) Stop() {
	f.once.Do(func() { close(f.stopped) })
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

func TestWatcherManager_LazyStartStop(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 4)
	factory, _ := countingFactory(t, procs)
	m := NewWatcherManager(context.Background(), factory)

	m.Acquire("w1")
	p := <-procs
	<-p.started // first subscriber STARTS the watcher

	m.Release("w1")
	<-p.stopped // last subscriber STOPS the watcher
}

func TestWatcherManager_RefcountNoDoubleStart(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 4)
	factory, calls := countingFactory(t, procs)
	m := NewWatcherManager(context.Background(), factory)

	m.Acquire("w1")
	p := <-procs
	<-p.started
	m.Acquire("w1") // second subscriber: no new watcher

	assert.EqualValues(t, 1, *calls)

	m.Release("w1") // still one subscriber left: alive
	select {
	case <-p.stopped:
		t.Fatal("watcher stopped while a subscriber remained")
	default:
	}

	m.Release("w1") // last subscriber leaves
	<-p.stopped
}

func TestWatcherManager_GitOnlyKeepsWatcherAlive(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 4)
	factory, calls := countingFactory(t, procs)
	m := NewWatcherManager(context.Background(), factory)

	m.Acquire("w1") // files subscriber starts it
	p := <-procs
	<-p.started
	m.Acquire("w1") // git subscriber shares the same refcount

	m.Release("w1") // files subscriber leaves; git keeps it alive
	select {
	case <-p.stopped:
		t.Fatal("watcher stopped while git subscriber remained")
	default:
	}

	m.Release("w1")
	<-p.stopped
	assert.EqualValues(t, 1, *calls)
}

func TestWatcherManager_BlankWsIDIgnored(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 1)
	factory, calls := countingFactory(t, procs)
	m := NewWatcherManager(context.Background(), factory)

	m.Acquire("")
	m.Release("")
	assert.EqualValues(t, 0, *calls)
}

func TestWatcherManager_ReleaseUnknownIsNoop(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 1)
	factory, _ := countingFactory(t, procs)
	m := NewWatcherManager(context.Background(), factory)

	m.Release("ghost") // must not panic
}

func TestWatcherManager_FactoryErrorDoesNotRegister(t *testing.T) {
	factory := func(
		_ context.Context,
		_ string,
	) (watcherProc, error) {
		return nil, errors.New("boom")
	}
	m := NewWatcherManager(context.Background(), factory)

	m.Acquire("w1")
	require.NotPanics(t, func() { m.Release("w1") })
}

func TestWatcherManager_StopAllStopsLiveWatchers(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 4)
	factory, _ := countingFactory(t, procs)
	m := NewWatcherManager(context.Background(), factory)

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

func TestWatcherManager_StopAllIdempotentAndEmptyNoop(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 4)
	factory, _ := countingFactory(t, procs)
	m := NewWatcherManager(context.Background(), factory)

	m.Acquire("w1")
	p := <-procs
	<-p.started

	m.StopAll()
	<-p.stopped
	require.NotPanics(t, m.StopAll) // second call is safe
}

func TestWatcherManager_StopAllOnEmptyIsNoop(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 4)
	factory, _ := countingFactory(t, procs)
	m := NewWatcherManager(context.Background(), factory)

	require.NotPanics(t, m.StopAll) // no live watcher: no-op
}

func TestWatcherManager_ReleaseAfterStopAllIsNoop(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 4)
	factory, _ := countingFactory(t, procs)
	m := NewWatcherManager(context.Background(), factory)

	m.Acquire("w1")
	p := <-procs
	<-p.started

	m.StopAll()
	<-p.stopped
	require.NotPanics(t, func() { m.Release("w1") })
}

func TestWatcherManager_AcquireAfterStopAllIsNoop(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 4)
	factory, calls := countingFactory(t, procs)
	m := NewWatcherManager(context.Background(), factory)

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

func TestWatcherManager_FlappingDoesNotLeak(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 8)
	factory, _ := countingFactory(t, procs)
	m := NewWatcherManager(context.Background(), factory)

	for i := 0; i < 4; i++ {
		m.Acquire("w1")
		p := <-procs
		<-p.started
		m.Release("w1")
		<-p.stopped
	}
}
