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
)

// Deleting a repo from Crowbar must never damage the user's own git repository
// (spec §3 P0-1). The repo row used to be deleted before its cascade ran, so
// every workspace was torn down with an EMPTY default branch: the default branch
// was force-deleted like any feature branch, and a locked protected-branch
// worktree was rm -rf'd with its uncommitted work.
//
// What Crowbar made goes: the branch it created for a workspace. What the user
// owns stays: the default branch, and every uncommitted change in a protected
// worktree git refuses to remove.
func TestRegression_DeleteRepo_KeepsTheUsersBranchesAndUncommittedWork(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h) // "main": the default branch, a locked managed worktree
	ctx := context.Background()

	mainWS, err := h.app.Repositories.Workspace.Get(ctx, imported.workspaceID)
	require.NoError(t, err)
	require.Equal(t, "locked", string(mainWS.Status), "the default branch's worktree is locked")
	unsaved := filepath.Join(mainWS.WorktreePath, "unsaved.txt")
	require.NoError(t, os.WriteFile(unsaved, []byte("uncommitted work\n"), 0o600))

	createChildWorkspace(t, h, imported, "feature/crowbar-made", imported.workspaceID)
	// A branch the user made themselves, which no workspace ever touched.
	runGit(t, imported.repoPath, "branch", "feature/users-own")

	reposWS := h.dial("/v0/projects/" + imported.projectID + "/repos")
	h.raw(http.MethodDelete, "/v0/projects/"+imported.projectID+"/repos/"+imported.repoID,
		nil, http.StatusAccepted).Body.Close()
	readUntil(t, reposWS, func(m map[string]any) bool {
		return m["id"] == imported.repoID && m["status"] == "deleted"
	})
	h.QuiesceReactors()

	assert.True(t, branchExists(imported.repoPath, "main"), "the default branch must survive its repo's removal")
	assert.True(t, branchExists(imported.repoPath, "feature/users-own"), "a branch the user made must survive")
	assert.False(t, branchExists(imported.repoPath, "feature/crowbar-made"),
		"the branch Crowbar created for its own workspace goes with it")
	assert.FileExists(t, unsaved, "uncommitted work in a protected worktree must never be destroyed")
}

func branchExists(repoPath, branch string) bool {
	//nolint:gosec // G204: test-only git invocation on a temp repo.
	return exec.Command("git", "-C", repoPath, "rev-parse", "--verify", "--quiet",
		"refs/heads/"+branch).Run() == nil
}
