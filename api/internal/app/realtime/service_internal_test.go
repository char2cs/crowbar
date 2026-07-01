package realtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestService_AcquireReleaseWatcherDrivesManager(t *testing.T) {
	s := newTestService(t, &recordingLifecycle{})

	procs := make(chan *fakeWatcherProc, 1)
	factory, _ := countingFactory(t, procs)
	s.watchers.factory = factory

	s.AcquireWatcher("w1")
	p := <-procs
	<-p.started

	s.watchers.mu.Lock()
	_, held := s.watchers.handles["w1"]
	s.watchers.mu.Unlock()
	require.True(t, held)

	s.ReleaseWatcher("w1")
	<-p.stopped

	s.watchers.mu.Lock()
	_, stillHeld := s.watchers.handles["w1"]
	s.watchers.mu.Unlock()
	assert.False(t, stillHeld)
}

func TestService_AcquireReleaseLSPDrivesManager(t *testing.T) {
	rec := &recordingLifecycle{}
	s := newTestService(t, rec)

	s.AcquireLSP("w1")
	s.ReleaseLSP("w1")

	assert.Equal(t, []string{"w1"}, rec.ensures)
	assert.Equal(t, []string{"w1"}, rec.shutdown)
}

func TestService_CloseStopsEverythingIdempotently(t *testing.T) {
	rec := &recordingLifecycle{}
	s := newTestService(t, rec)

	procs := make(chan *fakeWatcherProc, 1)
	factory, _ := countingFactory(t, procs)
	s.watchers.factory = factory

	s.AcquireWatcher("w1")
	p := <-procs
	<-p.started
	s.AcquireLSP("w1")

	require.NotPanics(t, s.Close)
	<-p.stopped
	require.NotPanics(t, s.Close) // second call is safe

	s.watchers.mu.Lock()
	remaining := len(s.watchers.handles)
	s.watchers.mu.Unlock()
	assert.Equal(t, 0, remaining)
}

func TestService_FactoryErrorReleaseIsNoop(t *testing.T) {
	repo := stubWorkspaceRepo{err: errors.New("boom")}
	s := New(
		context.Background(),
		hub.NewHub(),
		repo,
		enginegit.Engine(stubGitEngine{}),
		stubFSEngine{},
		NoopLSPLifecycle(),
		newFakePoller(),
		testPollInterval,
		time.Now,
	)

	s.AcquireWatcher("w1")
	require.NotPanics(t, func() { s.ReleaseWatcher("w1") })
}

func TestService_AcquireReleaseProviderPollDrivesManager(t *testing.T) {
	p := newFakePoller()
	s := newTestServiceWithPoller(t, &recordingLifecycle{}, p)

	s.AcquireProviderPoll("w1")
	select {
	case got := <-p.calls:
		assert.Equal(t, "w1", got)
	case <-time.After(time.Second):
		t.Fatal("AcquireProviderPoll did not start polling")
	}

	s.providerPoll.mu.Lock()
	_, held := s.providerPoll.handles["w1"]
	s.providerPoll.mu.Unlock()
	require.True(t, held)

	s.ReleaseProviderPoll("w1")

	s.providerPoll.mu.Lock()
	_, stillHeld := s.providerPoll.handles["w1"]
	s.providerPoll.mu.Unlock()
	assert.False(t, stillHeld)
}

func TestService_Close_StopsProviderPoll(t *testing.T) {
	p := newFakePoller()
	s := newTestServiceWithPoller(t, &recordingLifecycle{}, p)

	s.AcquireProviderPoll("w1")
	<-p.calls

	require.NotPanics(t, s.Close)

	s.providerPoll.mu.Lock()
	remaining := len(s.providerPoll.handles)
	s.providerPoll.mu.Unlock()
	assert.Equal(t, 0, remaining)
}

func newTestService(
	t *testing.T,
	lc LSPLifecycle,
) *Service {
	t.Helper()
	return newTestServiceWithPoller(t, lc, newFakePoller())
}

func newTestServiceWithPoller(
	t *testing.T,
	lc LSPLifecycle,
	poller ProviderPoller,
) *Service {
	t.Helper()
	return New(
		context.Background(),
		hub.NewHub(),
		stubWorkspaceRepo{ws: domain.Workspace{ID: "w1", WorktreePath: "/repo"}},
		enginegit.Engine(stubGitEngine{}),
		stubFSEngine{watcher: &enginefs.Watcher{}},
		lc,
		poller,
		testPollInterval,
		time.Now,
	)
}
