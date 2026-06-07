//go:build integration

package tests

import (
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
