//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegression_RepoFiledInHomeFolderDoesNotBlockSiblingRepoReorder(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	repoA := imported.repoID
	repoB := addSecondRepo(t, h, p)
	repoC := addSecondRepo(t, h, p)

	var homeWS struct {
		OwningChatID string `json:"owningChatId"`
	}
	h.get("/v0/projects/"+p+"/home", &homeWS)
	folder := createChatFolder(t, h, "/v0/projects/"+p+"/home", "hf", "")
	h.Quiesce()

	h.raw(http.MethodPatch, "/v0/projects/"+p+"/repos/"+repoB,
		map[string]any{"folderId": folder.ID, "order": 0}, http.StatusNoContent).Body.Close()
	h.Quiesce()
	assert.Equal(t, folder.ID, repoPlacement(t, h, p, repoB).FolderID)

	h.raw(http.MethodPatch, "/v0/projects/"+p+"/repos/"+repoC,
		map[string]any{"order": 0}, http.StatusNoContent).Body.Close()
	h.Quiesce()

	rows := homeTopLevel(t, h, p, homeWS.OwningChatID)
	assertDense(t, rows)
	assert.Equal(t, []string{repoC, repoA, folder.ID}, idsOf(rows),
		"the dragged repo lands at the front and the folder-filed repo stays out of the root level")
	assert.Equal(t, folder.ID, repoPlacement(t, h, p, repoB).FolderID,
		"a sibling's reorder must never re-root a repo filed in a folder")
}

func TestRegression_RepoFiledInHomeFolderCanReturnToTheRoot(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	repoA := imported.repoID
	repoB := addSecondRepo(t, h, p)

	var homeWS struct {
		OwningChatID string `json:"owningChatId"`
	}
	h.get("/v0/projects/"+p+"/home", &homeWS)
	folder := createChatFolder(t, h, "/v0/projects/"+p+"/home", "hf", "")
	h.Quiesce()

	h.raw(http.MethodPatch, "/v0/projects/"+p+"/repos/"+repoB,
		map[string]any{"folderId": folder.ID, "order": 0}, http.StatusNoContent).Body.Close()
	h.Quiesce()

	h.raw(http.MethodPatch, "/v0/projects/"+p+"/repos/"+repoB,
		map[string]any{"folderId": "", "order": 0}, http.StatusNoContent).Body.Close()
	h.Quiesce()

	placement := repoPlacement(t, h, p, repoB)
	require.Empty(t, placement.FolderID, "the repo is back at the project root")
	rows := homeTopLevel(t, h, p, homeWS.OwningChatID)
	assertDense(t, rows)
	assert.Equal(t, []string{repoB, repoA, folder.ID}, idsOf(rows))
}

func repoPlacement(t *testing.T, h *harness, projectID, repoID string) struct {
	FolderID string `json:"folderId"`
	Order    int    `json:"order"`
} {
	t.Helper()
	var repos []struct {
		ID       string `json:"id"`
		FolderID string `json:"folderId"`
		Order    int    `json:"order"`
	}
	h.get("/v0/projects/"+projectID+"/repos", &repos)
	for _, r := range repos {
		if r.ID == repoID {
			return struct {
				FolderID string `json:"folderId"`
				Order    int    `json:"order"`
			}{FolderID: r.FolderID, Order: r.Order}
		}
	}
	require.Failf(t, "repo not found", "repo %s missing from the project's repo list", repoID)
	return struct {
		FolderID string `json:"folderId"`
		Order    int    `json:"order"`
	}{}
}
