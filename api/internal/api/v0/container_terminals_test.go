//go:build integration

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

// terminalsTestContainers builds the app + engine containers used by the
// terminals snapshot test. It mirrors the helper in container_test.go but lives
// in-package so it can exercise the unexported terminalsSnapshot directly.
func terminalsTestContainers(
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

// TestTerminalsDef_SnapshotFromEngine proves the lifecycle topic's snapshot is
// derived from the in-memory engine registry (D6: no terminal_sessions view.db):
// a session created in the engine surfaces as an "active" DTO carrying the
// owning workspace's project/repo scope.
func TestTerminalsDef_SnapshotFromEngine(t *testing.T) {
	appContainer, engContainer := terminalsTestContainers(t)
	ctx := context.Background()

	worktree := t.TempDir()
	_, err := appContainer.Repositories.Workspace.Create(
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

	snap := terminalsSnapshot(appContainer, engContainer)
	require.NotNil(t, snap)
	assert.Empty(t, snap("p1/r1/w1"), "no sessions yet")

	sid, err := engContainer.Terminal.Create(ctx, "w1", worktree, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = engContainer.Terminal.Kill(ctx, sid) })

	got := snap("p1/r1/w1")
	require.Len(t, got, 1)
	assert.Equal(t, sid, got[0].ID)
	assert.Equal(t, "p1", got[0].ProjectID)
	assert.Equal(t, "r1", got[0].RepoID)
	assert.Equal(t, "w1", got[0].WorkspaceID)
	assert.Equal(t, "active", got[0].Status)
}
