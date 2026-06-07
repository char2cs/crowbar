package v0

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestContainer(
	watchers *WatcherManager,
	lsps *LSPManager,
) *Container {
	root, cancel := context.WithCancel(context.Background())
	watchers.root = root
	lsps.root = root
	return &Container{
		watchers:   watchers,
		lsps:       lsps,
		cancelRoot: cancel,
	}
}

func TestContainer_CloseStopsLiveWatcherAndLSP(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 4)
	factory, _ := countingFactory(t, procs)
	watchers := NewWatcherManager(context.Background(), factory)
	rec := &recordingLifecycle{}
	lsps := NewLSPManager(context.Background(), rec)
	c := newTestContainer(watchers, lsps)

	watchers.Acquire("w1")
	p := <-procs
	<-p.started
	lsps.Acquire("w1")

	c.Close()

	<-p.stopped
	assert.Equal(t, 1, rec.shutdownCount())

	watchers.mu.Lock()
	wHandles := len(watchers.handles)
	watchers.mu.Unlock()
	assert.Equal(t, 0, wHandles)
}

func TestContainer_CloseIdempotentAndEmpty(t *testing.T) {
	procs := make(chan *fakeWatcherProc, 4)
	factory, _ := countingFactory(t, procs)
	watchers := NewWatcherManager(context.Background(), factory)
	rec := &recordingLifecycle{}
	lsps := NewLSPManager(context.Background(), rec)
	c := newTestContainer(watchers, lsps)

	require.NotPanics(t, c.Close) // no live resources: no-op
	require.NotPanics(t, c.Close) // second call is safe
	assert.Equal(t, 0, rec.shutdownCount())
}
