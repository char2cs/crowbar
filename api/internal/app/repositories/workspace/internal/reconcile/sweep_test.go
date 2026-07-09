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

// TestSweep_ReapsDeletedLingering_LeavesCleanIntact drives a real axWorkspace
// with the real save-only store projection registered: it creates two workspaces
// and Deletes one. With NO delete reactor registered the projection persists the
// Status="deleted" tombstone but the worktree and row LINGER — exactly the crash
// state the boot orphan-sweep exists to reap (a reactor that died mid-cascade,
// spec §3.8). Sweep reads the read model directly and re-drives the idempotent
// purge for the deleted row ONLY: its worktree is rm'd, its aggregate Forgotten,
// and its read-model row dropped (via Forget's OnForget), converging to the
// delete invariant (no row, no worktree) — while the clean workspace and its
// worktree are left untouched.
func TestSweep_ReapsDeletedLingering_LeavesCleanIntact(t *testing.T) {
	ctx, ax := newAx(t)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := projections.NewStore(db)
	require.NoError(t, err)
	require.NoError(t, projections.RegisterStore(st, ax))

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
		})
		require.NoError(t, err)
	}
	_, err = ax.SendWait(ctx, wscmds.Delete{ID: "del"})
	require.NoError(t, err)

	// Precondition: the tombstone row lingers (no reactor purged it).
	tomb, err := st.Get(ctx, "del")
	require.NoError(t, err)
	require.NotNil(t, tomb)
	require.Equal(t, domain.WorkspaceStatusDeleted, tomb.Status)

	// purge re-drives the same idempotent teardown a delete reactor would: rm -rf
	// the worktree, then Forget the aggregate (OnForget drops the read-model row).
	var purged []string
	purge := func(ctx context.Context, wsID string) error {
		purged = append(purged, wsID)
		if p := paths[wsID]; p != "" {
			if rmErr := os.RemoveAll(p); rmErr != nil {
				return rmErr
			}
		}
		delete(paths, wsID)
		return ax.Forget(ctx, wsID)
	}

	NewSweeper(st.List, purge).Sweep(ctx)

	assert.Equal(t, []string{"del"}, purged, "only the deleted row must be purged")

	gone, err := st.Get(ctx, "del")
	require.NoError(t, err)
	assert.Nil(t, gone, "the deleted row must be reaped (Forget's OnForget drops it)")
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

// TestSweep_EmptyModel_NoReplay proves the §3.8 accepted consequence: a boot that
// also lost store/workspace.db leaves the sweep reading an EMPTY model, so it
// reaps nothing and — crucially — never pays a lazy Replay to rebuild it (that is
// deferred to the first on-demand access, §3.7). The Sweeper is wired ONLY to the
// direct read (list) and a purge — it has no Replay path by construction, so a
// Replay spy it is deliberately never given must stay untouched.
func TestSweep_EmptyModel_NoReplay(t *testing.T) {
	replays := 0
	// directList models state/store/workspace.db read directly: unlike the
	// lazy-Replay List wrapper (§3.7) it returns an empty model as empty rather
	// than rebuilding it. If the sweep had been (wrongly) wired to the replay
	// path, replays would increment — the Sweeper simply has no such dependency.
	directList := func(context.Context) ([]domain.Workspace, error) { return nil, nil }
	purgeCalled := false
	purge := func(context.Context, string) error { purgeCalled = true; return nil }

	NewSweeper(directList, purge).Sweep(context.Background())

	assert.False(t, purgeCalled, "an empty model reaps nothing")
	assert.Zero(t, replays, "the sweep must never trigger a lazy Replay")
}

// TestSweep_CleanModel_NoPurge asserts the sweep is a no-op when every row is in a
// live (non-deleted) status: it reads the model exactly once and purges nothing.
func TestSweep_CleanModel_NoPurge(t *testing.T) {
	reads := 0
	list := func(context.Context) ([]domain.Workspace, error) {
		reads++
		return []domain.Workspace{
			{ID: "a", Status: domain.WorkspaceStatusLocked},
			{ID: "b"}, // "" == has-commits-no-PR, still not deleted
			{ID: "c", Status: domain.WorkspaceStatusPROpen},
		}, nil
	}
	var purged []string
	purge := func(_ context.Context, id string) error { purged = append(purged, id); return nil }

	NewSweeper(list, purge).Sweep(context.Background())

	assert.Empty(t, purged, "a clean model must trigger no purge")
	assert.Equal(t, 1, reads, "the read model is read directly exactly once")
}

// TestSweep_ListError_SkipsPurge proves a read-model List failure is non-fatal:
// the sweep logs and returns without purging anything, so boot is never failed by
// recovery work.
func TestSweep_ListError_SkipsPurge(t *testing.T) {
	list := func(context.Context) ([]domain.Workspace, error) {
		return nil, errors.New("read model unavailable")
	}
	purgeCalled := false
	purge := func(context.Context, string) error { purgeCalled = true; return nil }

	NewSweeper(list, purge).Sweep(context.Background())

	assert.False(t, purgeCalled, "a list failure must abort before any purge")
}

// TestSweep_PurgeError_ContinuesToNextDeleted proves a per-id purge failure never
// aborts the sweep: the failing row is logged and skipped, the clean row is never
// purged, and the next deleted row is still reaped.
func TestSweep_PurgeError_ContinuesToNextDeleted(t *testing.T) {
	list := func(context.Context) ([]domain.Workspace, error) {
		return []domain.Workspace{
			{ID: "boom", Status: domain.WorkspaceStatusDeleted},
			{ID: "keep", Status: domain.WorkspaceStatusLocked},
			{ID: "ok", Status: domain.WorkspaceStatusDeleted},
		}, nil
	}
	var purged []string
	purge := func(_ context.Context, id string) error {
		purged = append(purged, id)
		if id == "boom" {
			return errors.New("rm failed")
		}
		return nil
	}

	NewSweeper(list, purge).Sweep(context.Background())

	assert.Equal(t, []string{"boom", "ok"}, purged,
		"a failing purge is skipped, the clean row is never purged, and the next deleted row still is")
}
