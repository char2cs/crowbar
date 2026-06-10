//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProjects_ImportThenList proves the import flow end to end: POST /v0/projects
// adopts a real git repo, GET /v0/projects lists it, and GET /v0/repos?projectId=
// surfaces the discovered repository.
func TestProjects_ImportThenList(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	var projects []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Path string `json:"path"`
	}
	h.get("/v0/projects", &projects)
	require.Len(t, projects, 1)
	assert.Equal(t, imported.projectID, projects[0].ID)
	assert.Equal(t, "demo", projects[0].Name)
	assert.Equal(t, imported.repoPath, projects[0].Path)

	repos := listRepos(t, h, imported.projectID)
	require.Len(t, repos, 1)
	assert.Equal(t, imported.repoID, repos[0].ID)
	assert.Equal(t, imported.repoPath, repos[0].Path)
}

// TestProjects_DeleteCascadesRecordsKeepsRealRepoOnDisk proves the delete flow
// end to end: import a project, create a crowbar-managed child workspace, then
// DELETE /v0/projects/:id. The project, its repos, and its workspaces vanish
// from every list; the user's real repository directory survives on disk while
// the crowbar-created child worktree directory is removed.
func TestProjects_DeleteCascadesRecordsKeepsRealRepoOnDisk(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)

	childPath := worktreePathOf(t, h, imported.workspaceID)
	require.NotEqual(t, imported.repoPath, childPath)
	require.DirExists(t, childPath, "child worktree must exist on disk before delete")

	var deleted struct {
		ID string `json:"id"`
	}
	h.del("/v0/projects/"+imported.projectID, nil, http.StatusOK, &deleted)
	assert.Equal(t, imported.projectID, deleted.ID)

	var projects []struct {
		ID string `json:"id"`
	}
	h.get("/v0/projects", &projects)
	assert.Empty(t, projects, "project record must be gone")

	repos := listRepos(t, h, imported.projectID)
	assert.Empty(t, repos, "repo records must be gone")

	var workspaces []workspaceDTO
	h.get("/v0/workspaces", &workspaces)
	assert.Empty(t, workspaces, "workspace records must be gone")

	assert.DirExists(t, imported.repoPath,
		"the user's real repository directory must never be deleted")
	assert.FileExists(t, imported.repoPath+"/README.md",
		"repo contents must survive project deletion")
	assert.NoDirExists(t, childPath,
		"the crowbar-created child worktree directory must be removed")
}

// TestProjects_DeleteMissing proves DELETE /v0/projects/:id returns the 404
// error envelope when the project does not exist.
func TestProjects_DeleteMissing(t *testing.T) {
	h := newHarness(t)

	resp := h.raw(http.MethodDelete, "/v0/projects/no-such-project", nil, http.StatusNotFound)
	_ = resp.Body.Close()
}

// worktreePathOf resolves a workspace's on-disk worktree path via the detail
// endpoint; the WorkspaceDTO carries the path directly.
func worktreePathOf(
	t *testing.T,
	h *harness,
	wsID string,
) string {
	t.Helper()
	var ws struct {
		WorktreePath string `json:"worktreePath"`
	}
	h.get("/v0/workspaces/"+wsID, &ws)
	require.NotEmpty(t, ws.WorktreePath)
	return ws.WorktreePath
}
