package resources

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeReleaser records the owners ReleaseWorkspace was called for.
type fakeReleaser struct {
	mu       sync.Mutex
	released []string
}

func (f *fakeReleaser) ReleaseWorkspace(
	_ context.Context,
	ownerID string,
) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released = append(f.released, ownerID)
}

func (f *fakeReleaser) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.released...)
}

// A client that drops with documents still open must not leak their servers.
func TestLSP_StopReleasesTheOwnersServers(t *testing.T) {
	rel := &fakeReleaser{}
	l := NewLSP(context.Background(), rel)

	require.NoError(t, l.Start("c1"))
	assert.Empty(t, rel.calls(), "a start runs no server: documents do")

	l.Stop("c1")
	assert.Equal(t, []string{"c1"}, rel.calls())
}

func TestLSP_StopOfAnOwnerNeverStartedIsANoop(t *testing.T) {
	rel := &fakeReleaser{}
	l := NewLSP(context.Background(), rel)

	l.Stop("ghost")

	assert.Empty(t, rel.calls())
}

func TestLSP_OwnersAreIndependent(t *testing.T) {
	rel := &fakeReleaser{}
	l := NewLSP(context.Background(), rel)
	require.NoError(t, l.Start("c1"))
	require.NoError(t, l.Start("c2"))

	l.Stop("c1")

	assert.Equal(t, []string{"c1"}, rel.calls())
}

func TestLSP_CloseReleasesEveryHeldOwnerAndRefusesLaterStarts(t *testing.T) {
	rel := &fakeReleaser{}
	l := NewLSP(context.Background(), rel)
	require.NoError(t, l.Start("c1"))
	require.NoError(t, l.Start("c2"))

	l.Close()
	l.Close()

	assert.ElementsMatch(t, []string{"c1", "c2"}, rel.calls())
	require.ErrorIs(t, l.Start("c3"), errClosed)
	l.Stop("c3")
	assert.Len(t, rel.calls(), 2)
}
