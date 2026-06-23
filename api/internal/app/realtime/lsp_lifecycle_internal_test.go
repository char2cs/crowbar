package realtime

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeReleaser records the workspaces ReleaseWorkspace was called for.
type fakeReleaser struct {
	mu       sync.Mutex
	released []string
}

func (f *fakeReleaser) ReleaseWorkspace(
	_ context.Context,
	wsID string,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released = append(f.released, wsID)
}

func (f *fakeReleaser) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.released...)
}

// TestLSPLifecycle_ShutdownReleasesWorkspace proves R11: the production
// lifecycle's Shutdown edge releases ALL of the workspace's language servers via
// the engine, and Ensure does no work (servers follow document sync).
func TestLSPLifecycle_ShutdownReleasesWorkspace(t *testing.T) {
	rel := &fakeReleaser{}
	lc := NewLSPLifecycle(rel)

	lc.Ensure(context.Background(), "w1")
	assert.Empty(t, rel.calls(), "Ensure must not touch the engine")

	lc.Shutdown(context.Background(), "w1")
	assert.Equal(t, []string{"w1"}, rel.calls(), "Shutdown must release the workspace")
}

// TestLSPLifecycle_LastUnsubscribeReleasesWorkspace proves the full R11 wiring:
// the LSPManager's last-unsubscribe (1->0) edge drives the production lifecycle,
// which releases the workspace's servers — the WS-drop path that the per-document
// DidClose decrement would otherwise miss on a force-quit/crash.
func TestLSPLifecycle_LastUnsubscribeReleasesWorkspace(t *testing.T) {
	rel := &fakeReleaser{}
	m := NewLSPManager(context.Background(), NewLSPLifecycle(rel))

	m.Acquire("w1")
	m.Acquire("w1") // second subscriber: still held
	require.Empty(t, rel.calls(), "workspace must not be released while a subscriber remains")

	m.Release("w1")
	require.Empty(t, rel.calls(), "still one subscriber: no release")

	m.Release("w1") // last subscriber drops: release the workspace
	assert.Equal(t, []string{"w1"}, rel.calls(), "last unsubscribe must release the workspace's servers")
}

// TestLSPLifecycle_StopAllReleasesEveryHeldWorkspace verifies graceful shutdown
// releases every workspace still subscribed.
func TestLSPLifecycle_StopAllReleasesEveryHeldWorkspace(t *testing.T) {
	rel := &fakeReleaser{}
	m := NewLSPManager(context.Background(), NewLSPLifecycle(rel))

	m.Acquire("w1")
	m.Acquire("w2")

	m.StopAll()

	got := rel.calls()
	assert.ElementsMatch(t, []string{"w1", "w2"}, got)
}
