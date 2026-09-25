//go:build integration

package crash_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// A repo delete records its intent before any teardown. A crash right after
// that write left a repo half-way to deleted with nothing to finish it (D5):
// the next boot now re-drives the same delete — its workspaces are retired and
// purged, and its row goes.
func TestCrash_RepoDeleteIntent_BootFinishesTheDelete(t *testing.T) {
	home := kit.TempHomeForTest(t)
	env1 := kit.BuildEnvAt(t, home)
	imported := env1.ImportRepo(t, "crash-repo-delete", "")
	const branch = "feature/finish-me"
	env1.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, branch, "")
	worktree := friendlyWorktree(env1, imported.ProjectID, imported.RepoPath, branch)
	require.True(t, kit.DirExists(t, worktree), "worktree must exist before delete")
	env1.Quiesce()

	env1.RecordRepoDeleteIntent(t, imported.RepoID)
	env1.CloseCrashing(t)

	env2, err := kit.NewEnvWithHome(home)
	require.NoError(t, err, "restart over the same home after a crash")
	defer env2.Close(t)
	env2.QuiesceReactors()

	require.False(t, env2.RepoRow(t, imported.RepoID), "boot must finish the repo delete")
	require.False(t, kit.DirExists(t, worktree), "and purge the repo's worktrees")
	require.True(t, kit.DirExists(t, imported.RepoPath), "the user's own checkout is never touched")
}
