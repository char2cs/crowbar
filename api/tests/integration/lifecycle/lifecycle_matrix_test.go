//go:build integration

package lifecycle_test

// lifecycle_matrix_test.go carries the restart/drain/rebuild slice of the Task
// 17 crash/recovery/rebuild/friendly-path integration matrix (spec §5 table).
// Unlike the LifecycleSuite (which auto-wires a single fresh Env per test),
// these cases drive the daemon restart primitive directly: BuildEnvAt over a
// caller-owned home, a teardown variant (graceful Close / crash), then a second
// NewEnvWithHome over the SAME home to prove durable read models survive. They
// deliberately share the package's existing TestMain (kit.Main).

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// listWorkspaceIDs returns the set of workspace ids the repo-scoped chat list
// reports (served from the durable store/workspace.db read model via each
// worktree-owning chat's own row) — the read-model replacement for the deleted
// GET .../workspaces list (spec §8 step 6).
func listWorkspaceIDs(t *testing.T, env *kit.Env, projectID, repoID string) map[string]bool {
	t.Helper()
	chats := env.ListChats(t, projectID, repoID)
	ids := make(map[string]bool, len(chats))
	for _, c := range chats {
		if id, _ := c["workspaceId"].(string); id != "" {
			ids[id] = true
		}
	}
	return ids
}

// TestLifecycleMatrix_GracefulRestartPersistsNoReplay covers spec §5 row
// "Graceful restart persists": after a graceful drain (Close), a second env over
// the same home reads the intact read model via GET, and NO replay was needed on
// boot. "No replay ran" is asserted through its observable proxy: the durable
// store/workspace.db is present and non-empty when the second env boots, so
// ListOrRebuild's len(rows)>0 fast path returns without ever entering the
// AggregateLister/Replay repair branch (spec §3.7, decision 7). The companion
// TestLifecycleMatrix_LazyReadModelRebuild proves replay fires ONLY when that
// durable model is empty, so a populated model boot is replay-free.
func TestLifecycleMatrix_GracefulRestartPersistsNoReplay(t *testing.T) {
	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "restart-persist", "")

	want := map[string]bool{}
	for _, b := range []string{"feature/a", "feature/b", "feature/c"} {
		want[env1.CreateWorkspace(t, imported.ProjectID, imported.RepoID, b)] = true
	}
	env1.Close(t) // graceful, ordered drain + WAL checkpoint + close.

	// The durable read model persisted to disk and is non-empty, so the restart
	// serves it directly (no replay).
	storeDB := filepath.Join(home, "state", "store", "workspace.db")
	info, err := os.Stat(storeDB)
	require.NoError(t, err, "durable read model must survive a graceful close")
	require.Positive(t, info.Size(), "durable read model must be non-empty on boot (no replay needed)")

	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home")
	defer env2.Close(t)

	got := listWorkspaceIDs(t, env2, imported.ProjectID, imported.RepoID)
	for id := range want {
		require.True(t, got[id], "workspace %s must survive a graceful restart via the durable read model", id)
	}
}

// TestLifecycleMatrix_GracefulDrainIntegrity covers spec §5 row "Graceful drain
// integrity": a graceful Close taken while mutations are in flight drains every
// asynx singleton and closes all DBs WITHIN the deadline (no hang, unlike
// quiver's unbounded drainWg.Wait), loses no committed writes, and a restart
// reads them all back (spec §3.8 shutdown, decision 11).
func TestLifecycleMatrix_GracefulDrainIntegrity(t *testing.T) {
	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "drain", "")

	const n = 6
	branches := make([]string, n)
	for i := range branches {
		branches[i] = "feature/drain-" + string(rune('a'+i))
	}
	// Commit n workspaces, then fire concurrent status-syncs so post-commit
	// projections are in flight when the drain begins. Sync is now addressed by
	// the OWNING CHAT (spec §8 step 6 deleted the :wsId-keyed route), so each
	// workspace's chat id is minted alongside it.
	ids := make([]string, n)
	chatIDs := make([]string, n)
	for i, b := range branches {
		ids[i], chatIDs[i] = env1.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, b, "")
	}
	var wg sync.WaitGroup
	for _, chatID := range chatIDs {
		wg.Add(1)
		go func(chatID string) {
			defer wg.Done()
			resp := env1.POST(t,
				"/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/chats/"+chatID+"/sync", nil)
			resp.Body.Close()
		}(chatID)
	}
	wg.Wait()

	// The graceful drain must return promptly — well under any hang threshold.
	start := time.Now()
	env1.Close(t)
	elapsed := time.Since(start)
	require.Less(t, elapsed, 20*time.Second, "graceful drain must be bounded, not hang (took %v)", elapsed)

	// No lost writes: every committed workspace is readable after restart.
	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home")
	defer env2.Close(t)
	got := listWorkspaceIDs(t, env2, imported.ProjectID, imported.RepoID)
	for _, id := range ids {
		require.True(t, got[id], "drain must not lose committed workspace %s", id)
	}
}

// TestLifecycleMatrix_LazyReadModelRebuild covers spec §5 row "Lazy read-model
// rebuild": with the durable store/workspace.db deleted, a restart's first List
// finds an empty read model over a NON-empty event log and heals it via a
// whole-model lazy Replay — the AggregateLister enumerates every id and Replay
// rebuilds each row (spec §3.7, decision 7). Startup itself never pays this; it
// fires only on the first post-loss List.
func TestLifecycleMatrix_LazyReadModelRebuild(t *testing.T) {
	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "rebuild", "")
	want := map[string]bool{}
	for _, b := range []string{"feature/r1", "feature/r2"} {
		want[env1.CreateWorkspace(t, imported.ProjectID, imported.RepoID, b)] = true
	}
	env1.Close(t)

	// Simulate a lost read model: delete store/workspace.db (+ WAL/SHM). The
	// append-only event log under state/events/ is the untouched source of truth.
	storeDB := filepath.Join(home, "state", "store", "workspace.db")
	require.True(t, kit.FileExists(t, storeDB), "read model must exist before deletion")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(storeDB + suffix)
	}
	require.False(t, kit.FileExists(t, storeDB), "read model must be gone before restart")

	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home")
	defer env2.Close(t)

	// The first List over the empty-but-log-non-empty model triggers the rebuild.
	got := listWorkspaceIDs(t, env2, imported.ProjectID, imported.RepoID)
	for id := range want {
		require.True(t, got[id], "lazy Replay must rebuild workspace %s from the event log", id)
	}
}
