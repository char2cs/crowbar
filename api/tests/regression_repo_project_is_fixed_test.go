//go:build integration

package tests

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A repository's project is fixed at import (spec §3 P0-3, invariant D6).
//
// Everything a repo owns on disk is keyed by its project: the managed worktrees
// (<home>/projects/<P>/<slug>/<branch>), the home checkout's chats tree, the
// per-workspace storage dirs and the repo's entity dir. Re-pointing the rows at
// another project moved none of it — worktrees stayed under projects/A, where a
// delete of A could reach them — and the per-workspace re-point was a
// non-atomic loop that a failure left half-moved. The move is refused: nothing
// changes, on disk or in any row.
func TestRegression_RepoMoveToAnotherProject_IsRefusedAndChangesNothing(t *testing.T) {
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
	before := workspaceIDs(listWorkspaces(t, h, imported.projectID, imported.repoID))
	require.NotEmpty(t, before)

	h.raw(http.MethodPatch, repoBase(imported),
		map[string]string{"projectId": otherID}, http.StatusConflict).Body.Close()
	h.Quiesce()

	assert.Equal(t, before, workspaceIDs(listWorkspaces(t, h, imported.projectID, imported.repoID)),
		"every workspace stays with the repo's project")
	var repos []repoDTO
	h.get("/v0/projects/"+imported.projectID+"/repos", &repos)
	found := false
	for _, r := range repos {
		found = found || r.ID == imported.repoID
	}
	assert.True(t, found, "the repo is still listed under its own project")
	h.raw(http.MethodGet, "/v0/projects/"+otherID+"/repos/"+imported.repoID, nil,
		http.StatusNotFound).Body.Close()
	assert.DirExists(t, childPath)
	assert.True(t, strings.Contains(filepath.ToSlash(childPath), "/projects/"+imported.projectID+"/"),
		"the worktree lives under its own project's directory (D6)")

	// Naming the project the repo is already in is not a move.
	h.raw(http.MethodPatch, repoBase(imported),
		map[string]string{"projectId": imported.projectID}, http.StatusNoContent).Body.Close()
}

func workspaceIDs(rows []workspaceDTO) []string {
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids
}
