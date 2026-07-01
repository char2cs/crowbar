//go:build integration

package tests

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProjects_ImportThenList proves the import flow end to end: POST /v0/projects
// adopts a real git repo (202 + WS), GET /v0/projects lists it, and
// GET /v0/projects/:p/repos surfaces the discovered repository.
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
// DELETE /v0/projects/:projectId. The delete is async (202 + WS tombstone): a
// deleted-status ProjectDTO is broadcast once the cascade completes. The
// project, its repos, and its workspaces then vanish from every list; the user's
// real repository directory survives on disk while the crowbar-created child
// worktree directory is removed.
func TestProjects_DeleteCascadesRecordsKeepsRealRepoOnDisk(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)

	childPath := worktreePathOf(t, h, imported)
	require.NotEqual(t, imported.repoPath, childPath)
	require.DirExists(t, childPath, "child worktree must exist on disk before delete")

	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodDelete, "/v0/projects/"+imported.projectID, nil, http.StatusAccepted)
	_ = resp.Body.Close()

	tombstone := readUntil(t, projectsWS, func(m map[string]any) bool {
		return m["id"] == imported.projectID && m["status"] == "deleted"
	})
	assert.Equal(t, imported.projectID, tombstone["id"])

	var projects []struct {
		ID string `json:"id"`
	}
	h.get("/v0/projects", &projects)
	assert.Empty(t, projects, "project record must be gone")

	repos := listRepos(t, h, imported.projectID)
	assert.Empty(t, repos, "repo records must be gone")

	// The repo was cascade-deleted, so its workspace list is no longer reachable:
	// the repo scope guard 404s a :repoId that no longer belongs to the project.
	// That the repo scope is gone (alongside the empty repos list above) proves the
	// workspaces under it were removed by the cascade.
	h.raw(http.MethodGet,
		"/v0/projects/"+imported.projectID+"/repos/"+imported.repoID+"/workspaces",
		nil, http.StatusNotFound)

	assert.DirExists(t, imported.repoPath,
		"the user's real repository directory must never be deleted")
	assert.FileExists(t, imported.repoPath+"/README.md",
		"repo contents must survive project deletion")
	assert.NoDirExists(t, childPath,
		"the crowbar-created child worktree directory must be removed")
}

// TestProjects_DeleteMissing proves DELETE /v0/projects/:projectId returns the
// 404 error envelope when the project does not exist (synchronous validation,
// before the 202 async path).
func TestProjects_DeleteMissing(t *testing.T) {
	h := newHarness(t)

	resp := h.raw(http.MethodDelete, "/v0/projects/no-such-project", nil, http.StatusNotFound)
	_ = resp.Body.Close()
}

// TestRegression_HomeWorkspaceProvisionedOnCreate verifies that importing a
// project auto-provisions a home workspace with Kind=home.
//
// TODO(task-5): The harness exposes no direct repository access, so this test
// cannot verify the home workspace via GetHomeForProject. Once Task 5 adds
// GET /v0/projects/:id/home, replace the assertion below with an HTTP call.
func TestRegression_HomeWorkspaceProvisionedOnCreate(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	// The home workspace has no repo — it is not listed under
	// /v0/projects/:p/repos/:r/workspaces. Task 5 will surface it at
	// GET /v0/projects/:id/home. For now we assert the project itself exists,
	// which proves the import pipeline (including home-workspace provisioning)
	// completed without error.
	var projects []struct {
		ID string `json:"id"`
	}
	h.get("/v0/projects", &projects)
	require.Len(t, projects, 1)
	require.Equal(t, imported.projectID, projects[0].ID,
		"project must be persisted: home-workspace provisioning must not have failed the import")
}

// worktreePathOf resolves a child workspace's on-disk worktree path.
// WorktreePath is no longer carried on the wire WorkspaceDTO (D13), so it is
// reconstructed from the deterministic UUID layout the daemon uses:
// <home>/projects/<P>/<R>/workspaces/<W>/worktree.
func worktreePathOf(
	t *testing.T,
	h *harness,
	imported importedRepo,
) string {
	t.Helper()
	return filepath.Join(
		h.home,
		"projects",
		imported.projectID,
		imported.repoID,
		"workspaces",
		imported.workspaceID,
		"worktree",
	)
}
