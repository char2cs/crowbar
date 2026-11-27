//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

func repoBase(
	imported importedRepo,
) string {
	return "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID
}

func TestRegression_RepoListIsScopedToTheProjectInThePath(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	otherPath := gitRepoWithCommit(t)
	projectsWS := watchEntities(t, h, stream.KindProject)
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "second", "path": otherPath}, http.StatusAccepted)
	_ = resp.Body.Close()
	other := projectsWS.read(t, func(m map[string]any) bool {
		return m["path"] == otherPath
	})
	otherID, _ := other["id"].(string)
	require.NotEmpty(t, otherID)
	require.NotEqual(t, imported.projectID, otherID)

	h.Quiesce()

	var mine []repoDTO
	h.get("/v0/projects/"+imported.projectID+"/repos", &mine)
	require.NotEmpty(t, mine, "precondition: the first project has a repo")
	for _, r := range mine {
		assert.Equal(t, imported.projectID, r.ProjectID,
			"repo %s belongs to %s but was listed under %s", r.ID, r.ProjectID, imported.projectID)
	}

	var theirs []repoDTO
	h.get("/v0/projects/"+otherID+"/repos", &theirs)
	for _, r := range theirs {
		assert.Equal(t, otherID, r.ProjectID,
			"repo %s belongs to %s but was listed under %s", r.ID, r.ProjectID, otherID)
	}

	for _, r := range theirs {
		assert.NotEqual(t, imported.repoID, r.ID,
			"the other project listed a repo that is not its own")
	}
}
