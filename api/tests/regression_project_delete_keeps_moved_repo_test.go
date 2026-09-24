//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Moving a repo to project B leaves its worktrees where they were created,
// under projects/A. Deleting project A then rm -rf'd projects/A — B's
// worktrees and chats with it (spec §3 P0-3, invariant D6). A project delete
// removes only what the project owns.
func TestRegression_DeleteProject_KeepsTheWorktreesOfARepoMovedAway(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	childPath := worktreePathOf(t, h, imported)
	require.DirExists(t, childPath)

	otherPath := gitRepoWithCommit(t)
	projectsWS := h.dial("/v0/projects")
	h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "other", "path": otherPath}, http.StatusAccepted).Body.Close()
	other := readUntil(t, projectsWS, func(m map[string]any) bool { return m["path"] == otherPath })
	otherID, _ := other["id"].(string)
	require.NotEmpty(t, otherID)
	h.Quiesce()

	h.raw(http.MethodPatch, repoBase(imported),
		map[string]string{"projectId": otherID}, http.StatusNoContent).Body.Close()
	h.Quiesce()

	h.raw(http.MethodDelete, "/v0/projects/"+imported.projectID, nil, http.StatusAccepted).Body.Close()
	readUntil(t, projectsWS, func(m map[string]any) bool {
		return m["id"] == imported.projectID && m["status"] == "deleted"
	})
	h.QuiesceReactors()

	assert.DirExists(t, childPath, "the moved repo's worktree belongs to project B and must survive")
	found := false
	for _, ws := range listWorkspaces(t, h, otherID, imported.repoID) {
		if ws.ID == imported.workspaceID {
			found = true
		}
	}
	assert.True(t, found, "and so must its workspace")
}
