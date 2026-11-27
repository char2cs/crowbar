//go:build integration

package lifecycle_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

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

func TestLifecycleMatrix_GracefulRestartPersistsNoReplay(t *testing.T) {
	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "restart-persist", "")

	want := map[string]bool{}
	for _, b := range []string{"feature/a", "feature/b", "feature/c"} {
		want[env1.CreateWorkspace(t, imported.ProjectID, imported.RepoID, b)] = true
	}
	env1.Close(t)

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

func TestLifecycleMatrix_GracefulDrainIntegrity(t *testing.T) {
	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "drain", "")

	const n = 6
	branches := make([]string, n)
	for i := range branches {
		branches[i] = "feature/drain-" + string(rune('a'+i))
	}
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

	start := time.Now()
	env1.Close(t)
	elapsed := time.Since(start)
	require.Less(t, elapsed, 20*time.Second, "graceful drain must be bounded, not hang (took %v)", elapsed)

	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home")
	defer env2.Close(t)
	got := listWorkspaceIDs(t, env2, imported.ProjectID, imported.RepoID)
	for _, id := range ids {
		require.True(t, got[id], "drain must not lose committed workspace %s", id)
	}
}

func TestLifecycleMatrix_LazyReadModelRebuild(t *testing.T) {
	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "rebuild", "")
	want := map[string]bool{}
	for _, b := range []string{"feature/r1", "feature/r2"} {
		want[env1.CreateWorkspace(t, imported.ProjectID, imported.RepoID, b)] = true
	}
	env1.Close(t)

	storeDB := filepath.Join(home, "state", "store", "workspace.db")
	require.True(t, kit.FileExists(t, storeDB), "read model must exist before deletion")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(storeDB + suffix)
	}
	require.False(t, kit.FileExists(t, storeDB), "read model must be gone before restart")

	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home")
	defer env2.Close(t)

	got := listWorkspaceIDs(t, env2, imported.ProjectID, imported.RepoID)
	for id := range want {
		require.True(t, got[id], "lazy Replay must rebuild workspace %s from the event log", id)
	}
}
