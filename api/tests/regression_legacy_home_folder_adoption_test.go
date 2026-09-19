//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A legacy home folder (HomeID "") is adopted by the FIRST home that lists
// it unless its own rows name a home. A folder holding only repo header rows
// names none the adopter can see: homeOfRow answers a repo by
// repoMemberIDsForHome(THE CALLER's home), so project B's repo inside B's
// folder reads as "not mine" from project A — and A adopts B's folder.
func TestRegression_LegacyHomeFolderHoldingAnotherProjectsRepoIsStolenByFirstLister(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	projectA := createProjectForHome(t, h)
	imported := importProject(t, h)
	projectB := imported.projectID
	baseA := "/v0/projects/" + projectA + "/home"
	baseB := "/v0/projects/" + projectB + "/home"

	legacy := createChatFolder(t, h, baseB, "b-repos", "")
	h.raw(http.MethodPatch, "/v0/projects/"+projectB+"/repos/"+imported.repoID,
		map[string]any{"folderId": legacy.ID}, http.StatusNoContent).Body.Close()
	h.Quiesce()

	ctx := context.Background()
	f, err := h.app.GORM.Folders.FindByKey(ctx, legacy.ID)
	require.NoError(t, err)
	require.NotNil(t, f)
	f.HomeID = ""
	require.NoError(t, h.app.GORM.Folders.Save(ctx, *f))

	// Project A's sidebar seeds first (the active project, or simply the
	// first subscription to answer).
	inA := listChatFolders(t, h, baseA)
	_, stolen := chatFolderByID(inA, legacy.ID)
	assert.False(t, stolen, "project A must not adopt a folder holding only project B's repo")

	inB := listChatFolders(t, h, baseB)
	_, kept := chatFolderByID(inB, legacy.ID)
	assert.True(t, kept, "project B must still list the folder its repo is filed in")

	adopted, err := h.app.GORM.Folders.FindByKey(ctx, legacy.ID)
	require.NoError(t, err)
	var homeB struct {
		ID string `json:"id"`
	}
	h.get(baseB, &homeB)
	assert.Equal(t, homeB.ID, adopted.HomeID, "the folder must be recorded against project B's home")
}
