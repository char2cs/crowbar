//go:build integration

package tests

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

func TestRegression_DeleteRepo_KeepsTheUsersBranchesAndUncommittedWork(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	ctx := context.Background()

	mainWS, err := h.app.Repositories.Workspace.Get(ctx, imported.workspaceID)
	require.NoError(t, err)
	require.Equal(t, "locked", string(mainWS.Status), "the default branch's worktree is locked")
	unsaved := filepath.Join(mainWS.WorktreePath, "unsaved.txt")
	require.NoError(t, os.WriteFile(unsaved, []byte("uncommitted work\n"), 0o600))

	createChildWorkspace(t, h, imported, "feature/crowbar-made", imported.workspaceID)
	runGit(t, imported.repoPath, "branch", "feature/users-own")

	reposWS := watchEntities(t, h, stream.KindRepo)
	require.NoError(t, h.raw(http.MethodDelete, "/v0/projects/"+imported.projectID+"/repos/"+imported.repoID,
		nil, http.StatusAccepted).Body.Close())
	reposWS.read(t, func(m map[string]any) bool {
		return m["id"] == imported.repoID && m["gone"] == true
	})
	h.QuiesceReactors()

	assert.True(t, branchExists(t, imported.repoPath, "main"), "the default branch must survive its repo's removal")
	assert.True(t, branchExists(t, imported.repoPath, "feature/users-own"), "a branch the user made must survive")
	assert.False(t, branchExists(t, imported.repoPath, "feature/crowbar-made"),
		"the branch Crowbar created for its own workspace goes with it")
	assert.FileExists(t, unsaved, "uncommitted work in a protected worktree must never be destroyed")
}

func branchExists(t *testing.T, repoPath, branch string) bool {
	t.Helper()
	return exec.CommandContext(t.Context(), "git", "-C", repoPath, "rev-parse", "--verify", "--quiet",
		"refs/heads/"+branch).Run() == nil
}
