package reconcile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	wscmds "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/store/projections"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestSweep_ReapsDeletedLingering_LeavesCleanIntact(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := projections.NewStore(db)
	require.NoError(t, err)
	ctx, ax := newAx(t, st)

	root := t.TempDir()
	paths := map[string]string{
		"del":   filepath.Join(root, "del"),
		"clean": filepath.Join(root, "clean"),
	}
	for _, p := range paths {
		require.NoError(t, os.MkdirAll(filepath.Join(p, "sub"), 0o755))
	}

	for _, id := range []string{"del", "clean"} {
		_, err = ax.SendWait(ctx, wscmds.CreateWorkspace{
			ID: id, RepoID: "r", ProjectID: "p", Branch: "main",
			WorktreePath: paths[id], Now: time.Unix(1, 0).UTC(),
			Provisioning: domain.WorkspaceProvisioned,
		})
		require.NoError(t, err)
	}
	_, err = ax.SendWait(ctx, wscmds.Delete{ID: "del"})
	require.NoError(t, err)

	tomb, err := st.Get(ctx, "del")
	require.NoError(t, err)
	require.NotNil(t, tomb)
	require.Equal(t, domain.WorkspaceStatusDeleted, tomb.Status)

	var purged []string
	purge := func(ctx context.Context, tomb domain.Workspace) error {
		purged = append(purged, tomb.ID)
		if p := paths[tomb.ID]; p != "" {
			if rmErr := os.RemoveAll(p); rmErr != nil {
				return rmErr
			}
		}
		delete(paths, tomb.ID)
		return ax.Forget(ctx, tomb.ID)
	}

	NewSweeper(func(ctx context.Context) ([]domain.Workspace, error) { return st.List(ctx, leaveOut) }, purge).Sweep(ctx, noPurged)

	assert.Equal(t, []string{"del"}, purged, "only the deleted row must be purged")

	gone, err := st.Get(ctx, "del")
	require.NoError(t, err)
	assert.Nil(t, gone, "the deleted row must be reaped (Forget's marker drops it)")
	_, statErr := os.Stat(filepath.Join(root, "del"))
	assert.True(t, os.IsNotExist(statErr), "the lingering worktree must be rm'd")
	exists, err := ax.Exists(ctx, "del")
	require.NoError(t, err)
	assert.False(t, exists, "the aggregate must be Forgotten")

	stillThere, err := st.Get(ctx, "clean")
	require.NoError(t, err)
	assert.NotNil(t, stillThere, "the clean workspace row must survive")
	_, statErr = os.Stat(filepath.Join(root, "clean"))
	assert.NoError(t, statErr, "the clean worktree must survive")
}

func TestSweep_EmptyModel_NoReplay(t *testing.T) {
	replays := 0
	directList := func(context.Context) ([]domain.Workspace, error) { return nil, nil }
	purgeCalled := false
	purge := func(context.Context, domain.Workspace) error { purgeCalled = true; return nil }

	NewSweeper(directList, purge).Sweep(context.Background(), noPurged)

	assert.False(t, purgeCalled, "an empty model reaps nothing")
	assert.Zero(t, replays, "the sweep must never trigger a lazy Replay")
}

func TestSweep_CleanModel_NoPurge(t *testing.T) {
	reads := 0
	list := func(context.Context) ([]domain.Workspace, error) {
		reads++
		return []domain.Workspace{
			{ID: "a", Status: domain.WorkspaceStatusLocked},
			{ID: "b"},
			{ID: "c", Status: domain.WorkspaceStatusPROpen},
		}, nil
	}
	var purged []string
	purge := func(_ context.Context, ws domain.Workspace) error { purged = append(purged, ws.ID); return nil }

	NewSweeper(list, purge).Sweep(context.Background(), noPurged)

	assert.Empty(t, purged, "a clean model must trigger no purge")
	assert.Equal(t, 1, reads, "the read model is read directly exactly once")
}

func TestSweep_ListError_SkipsPurge(t *testing.T) {
	list := func(context.Context) ([]domain.Workspace, error) {
		return nil, errors.New("read model unavailable")
	}
	purgeCalled := false
	purge := func(context.Context, domain.Workspace) error { purgeCalled = true; return nil }

	NewSweeper(list, purge).Sweep(context.Background(), noPurged)

	assert.False(t, purgeCalled, "a list failure must abort before any purge")
}

func TestSweep_PurgeError_ContinuesToNextDeleted(t *testing.T) {
	list := func(context.Context) ([]domain.Workspace, error) {
		return []domain.Workspace{
			{ID: "boom", Status: domain.WorkspaceStatusDeleted},
			{ID: "keep", Status: domain.WorkspaceStatusLocked},
			{ID: "ok", Status: domain.WorkspaceStatusDeleted},
		}, nil
	}
	var purged []string
	purge := func(_ context.Context, ws domain.Workspace) error {
		purged = append(purged, ws.ID)
		if ws.ID == "boom" {
			return errors.New("rm failed")
		}
		return nil
	}

	var finished []string
	NewSweeper(list, purge).Sweep(context.Background(), func(_ context.Context, id string) { finished = append(finished, id) })

	assert.Equal(t, []string{"ok"}, finished, "only a purge that completed is reported finished")
	assert.Equal(t, []string{"boom", "ok"}, purged,
		"a failing purge is skipped, the clean row is never purged, and the next deleted row still is")
}

func noPurged(context.Context, string)                            {}
func leaveOut(context.Context, string) (*domain.Workspace, error) { return nil, nil }
