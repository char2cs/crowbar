package v0

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app"
	workspacerepo "github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// newAppAndEngineForSnapshot mirrors newAppForSnapshot but also returns the
// engine.Container terminalsSnapshot and lspSnapshot need directly (unlike the
// other snapshot sources, which only ever read through appContainer).
func newAppAndEngineForSnapshot(
	t *testing.T,
) (*app.Container, *engine.Container) {
	t.Helper()
	ctx := context.Background()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	a, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)
	return a, eng
}

// TestTerminalsSnapshot_NilEngineReturnsNil covers the guard at the top of
// terminalsSnapshot: a container built without the terminal engine wired
// (nil Container, or Container.Terminal nil) must not construct a source
// function at all, matching lspSnapshot's identical guard for an absent LSP
// engine.
func TestTerminalsSnapshot_NilEngineReturnsNil(t *testing.T) {
	a, _ := newAppAndEngineForSnapshot(t)

	assert.Nil(t, terminalsSnapshot(a, nil))
	assert.Nil(t, terminalsSnapshot(a, &engine.Container{}))
}

// TestTerminalsSnapshot_ScopeWithNoWorkspaceSegmentReturnsNil covers a repo- or
// project-level subscription (no workspace segment): terminals are always
// workspace-scoped, so such a scope must yield nil rather than attempting to
// enumerate every workspace's sessions.
func TestTerminalsSnapshot_ScopeWithNoWorkspaceSegmentReturnsNil(t *testing.T) {
	a, eng := newAppAndEngineForSnapshot(t)
	snap := terminalsSnapshot(a, eng)
	require.NotNil(t, snap)

	assert.Nil(t, snap("p1/r1"))
	assert.Nil(t, snap("p1/r1/"))
}

// TestTerminalsSnapshot_ListsLiveSessionForWorkspace is the ordinary case: a
// session created directly in the engine registry (D6: terminals are
// ephemeral, no view.db) must appear in the snapshot for its owning
// workspace's scope, carrying the project/repo stamped from the scope string
// (not from the session itself, which knows nothing about project/repo).
func TestTerminalsSnapshot_ListsLiveSessionForWorkspace(t *testing.T) {
	a, eng := newAppAndEngineForSnapshot(t)
	ctx := context.Background()

	worktree := t.TempDir()
	_, err := a.Repositories.Workspace.Create(
		ctx,
		workspacerepo.CreateInput{
			ID:           "w1",
			RepoID:       "r1",
			ProjectID:    "p1",
			Branch:       "main",
			WorktreePath: worktree,
		},
		time.Now().UTC(),
	)
	require.NoError(t, err)

	snap := terminalsSnapshot(a, eng)
	require.NotNil(t, snap)
	assert.Empty(t, snap("p1/r1/w1"), "no sessions yet")

	sid, err := eng.Terminal.Create(ctx, "w1", worktree, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Terminal.Kill(ctx, sid) })

	got := snap("p1/r1/w1")
	require.Len(t, got, 1)
	assert.Equal(t, sid, got[0].ID)
	assert.Equal(t, "p1", got[0].ProjectID)
	assert.Equal(t, "r1", got[0].RepoID)
	assert.Equal(t, "w1", got[0].WorkspaceID)
}
