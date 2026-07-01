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
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// threadsTestContainer builds the app container used by the threads snapshot
// test, mirroring terminalsTestContainers but in-package so it can exercise the
// unexported threadsSnapshot directly.
func threadsTestContainer(
	t *testing.T,
) *app.Container {
	t.Helper()
	ctx := context.Background()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	a, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)
	return a
}

// TestThreadsDef_ScopedSnapshotFromAggregate proves the Threads snapshot parses
// the connecting client's hierarchical scope (p/r/w), resolves the workspace id,
// and lists ONLY that workspace's threads from the global ReviewThread aggregate
// via ListByWorkspace — never a global enumeration — stamping the project/repo
// from the scope onto each ThreadDTO.
func TestThreadsDef_ScopedSnapshotFromAggregate(t *testing.T) {
	appContainer := threadsTestContainer(t)
	ctx := context.Background()

	snap := threadsSnapshot(appContainer)
	require.NotNil(t, snap)
	assert.Empty(t, snap("p1/r1/w1"), "no threads yet")

	opened, err := appContainer.Repositories.ReviewThread.Open(
		ctx,
		reviewthread.OpenInput{
			ID:         "t1",
			WsID:       "w1",
			FilePath:   "main.go",
			LineNumber: 7,
			Side:       domain.ReviewSideRight,
			MessageID:  "m1",
			Body:       "root comment",
		},
		time.Now().UTC(),
	)
	require.NoError(t, err)
	require.Equal(t, "t1", opened.ID)

	got := snap("p1/r1/w1")
	require.Len(t, got, 1)
	assert.Equal(t, "t1", got[0].ID)
	assert.Equal(t, "p1", got[0].ProjectID)
	assert.Equal(t, "r1", got[0].RepoID)
	assert.Equal(t, "w1", got[0].WorkspaceID)
	assert.Equal(t, "main.go", got[0].FilePath)
	assert.Equal(t, 7, got[0].Line)
	assert.Equal(t, "root comment", got[0].Body)

	// A different workspace scope sees none of w1's threads.
	assert.Empty(t, snap("p1/r1/w2"))
}

// TestThreadsDef_SnapshotNilWithoutWorkspaceScope proves a repo- or project-level
// subscription (no workspace segment) yields nil: threads are workspace-scoped,
// so the snapshot never enumerates across workspaces.
func TestThreadsDef_SnapshotNilWithoutWorkspaceScope(t *testing.T) {
	appContainer := threadsTestContainer(t)
	snap := threadsSnapshot(appContainer)
	require.NotNil(t, snap)

	assert.Nil(t, snap(""))
	assert.Nil(t, snap("p1"))
	assert.Nil(t, snap("p1/r1"))
}
