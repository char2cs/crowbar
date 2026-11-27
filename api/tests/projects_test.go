//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

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

func TestProjects_DeleteCascadesRecordsKeepsRealRepoOnDisk(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)

	childPath := worktreePathOf(t, h, imported)
	require.NotEqual(t, imported.repoPath, childPath)
	require.DirExists(t, childPath, "child worktree must exist on disk before delete")

	projectsWS := watchEntities(t, h, stream.KindProject)
	resp := h.raw(http.MethodDelete, "/v0/projects/"+imported.projectID, nil, http.StatusAccepted)
	_ = resp.Body.Close()

	tombstone := projectsWS.read(t, func(m map[string]any) bool {
		return m["id"] == imported.projectID && m["gone"] == true
	})
	assert.Equal(t, imported.projectID, tombstone["id"])

	var projects []struct {
		ID string `json:"id"`
	}
	h.get("/v0/projects", &projects)
	assert.Empty(t, projects, "project record must be gone")

	repos := listRepos(t, h, imported.projectID)
	assert.Empty(t, repos, "repo records must be gone")

	h.raw(http.MethodGet,
		"/v0/projects/"+imported.projectID+"/repos/"+imported.repoID+"/chats",
		nil, http.StatusNotFound)

	assert.DirExists(t, imported.repoPath,
		"the user's real repository directory must never be deleted")
	assert.FileExists(t, imported.repoPath+"/README.md",
		"repo contents must survive project deletion")
	assert.NoDirExists(t, childPath,
		"the crowbar-created child worktree directory must be removed")
}

func TestProjects_DeleteMissing(t *testing.T) {
	h := newHarness(t)

	resp := h.raw(http.MethodDelete, "/v0/projects/no-such-project", nil, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestRegression_HomeWorkspaceProvisionedOnCreate(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	var projects []struct {
		ID string `json:"id"`
	}
	h.get("/v0/projects", &projects)
	require.Len(t, projects, 1)
	require.Equal(t, imported.projectID, projects[0].ID,
		"project must be persisted: home-workspace provisioning must not have failed the import")
}

func worktreePathOf(
	t *testing.T,
	h *harness,
	imported importedRepo,
) string {
	t.Helper()
	return workspaceWorktreePath(t, h, imported)
}
