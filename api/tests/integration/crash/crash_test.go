//go:build integration

package crash_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(m *testing.M) {
	kit.Main(m)
}

func friendlyWorktree(env *kit.Env, projectID, repoPath, branch string) string {
	return filepath.Join(env.HomeDir(), "projects", projectID, filepath.Base(repoPath), branch)
}

func worktreeStatus(t *testing.T, env *kit.Env, projectID, repoID, wsID string) (string, bool) {
	t.Helper()
	row, ok := env.WorktreeChats(t, projectID, repoID)[wsID]
	if !ok {
		return "", false
	}
	status, _ := row["status"].(string)
	return status, true
}

func TestCrash_CommittedStateSurvivesAbruptKill(t *testing.T) {
	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "crash-durable", "")
	const branch = "feature/durable"
	wsID, _ := env1.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, branch, "")
	worktree := friendlyWorktree(env1, imported.ProjectID, imported.RepoPath, branch)
	require.True(t, kit.DirExists(t, worktree), "worktree must be provisioned before the crash")

	env1.Quiesce()

	env1.CloseCrashing(t)

	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home after a crash")
	defer env2.Close(t)

	status, present := worktreeStatus(t, env2, imported.ProjectID, imported.RepoID, wsID)
	require.True(t, present, "committed workspace must survive an abrupt kill")
	require.NotEqual(t, "deleted", status, "a committed non-deleted workspace must not be reaped by recovery")
	require.True(t, kit.DirExists(t, worktree), "provisioned worktree must survive an abrupt kill")
}

func TestCrash_ProviderDriftWhileDown_ResyncOnRestart(t *testing.T) {
	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "drift", "")
	wsID, _ := env1.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/drift", "")

	env1.PushProviderState(t, wsID, kit.ProviderState{
		HasPR:    true,
		PRStatus: "open",
		PRUrl:    "https://example.test/pr/7",
		PRTitle:  "feat: drift",
	})
	env1.Quiesce()
	s, ok := worktreeStatus(t, env1, imported.ProjectID, imported.RepoID, wsID)
	require.True(t, ok, "workspace must be present before the daemon goes down")
	require.Equal(t, "pr-open", s, "workspace must reach pr-open before the daemon goes down")
	env1.Close(t)

	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home")
	defer env2.Close(t)

	watcher := env2.HoldProjects(t)
	env2.PushProviderState(t, wsID, kit.ProviderState{
		HasPR:    true,
		PRStatus: "merged",
		PRUrl:    "https://example.test/pr/7",
		PRTitle:  "feat: drift",
	})
	merged := kit.WaitForWorkspaceState(t, watcher, wsID, "pr-merged", 5*time.Second)
	require.Equal(t, wsID, merged["id"])

	status, present := worktreeStatus(t, env2, imported.ProjectID, imported.RepoID, wsID)
	require.True(t, present)
	require.Equal(t, "pr-merged", status, "restarted read model must reflect the provider drift observed on re-poll")
}

func TestCrash_DeleteConvergesToInvariant(t *testing.T) {
	env := kit.BuildEnv(t)
	imported := env.ImportRepo(t, "delete", "")
	const branch = "feature/delete-me"
	wsID, _ := env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, branch, "")
	worktree := friendlyWorktree(env, imported.ProjectID, imported.RepoPath, branch)
	require.True(t, kit.DirExists(t, worktree), "worktree must exist before delete")

	watcher := env.HoldProjects(t)
	env.DeleteWorkspaceCascade(t, wsID)
	kit.WaitForWorkspaceState(t, watcher, wsID, "deleted", 10*time.Second)

	env.QuiesceReactors()
	_, present := env.WorkspaceRow(t, imported.ProjectID, imported.RepoID, wsID)
	require.False(t, present, "delete must converge to no read-model row")
	require.False(t, kit.DirExists(t, worktree), "delete must converge to no worktree on disk")
}

func TestCrash_DeleteMidCascade_BootSweepReaps(t *testing.T) {
	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "crash-delete", "")
	const branch = "feature/reap-me"
	wsID, _ := env1.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, branch, "")
	worktree := friendlyWorktree(env1, imported.ProjectID, imported.RepoPath, branch)
	require.True(t, kit.DirExists(t, worktree), "worktree must exist before delete")

	env1.HoldReactors()
	env1.DeleteWorkspaceCascade(t, wsID)
	env1.Quiesce()

	status, present := env1.WorkspaceRow(t, imported.ProjectID, imported.RepoID, wsID)
	require.True(t, present, "precondition: the deleted row must be durable before the crash")
	require.Equal(t, "deleted", status,
		"precondition: the tombstone must be PERSISTED before the crash — it is the only thing "+
			"the next boot's sweep can find")

	env1.CloseCrashing(t)

	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home after a crash")
	defer env2.Close(t)

	_, stillThere := env2.WorkspaceRow(t, imported.ProjectID, imported.RepoID, wsID)
	require.False(t, stillThere, "boot sweep must reap the crash-orphaned deleted row")
	require.False(t, kit.DirExists(t, worktree), "boot sweep must reap the lingering worktree")
}
