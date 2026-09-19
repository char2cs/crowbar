//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reported live against production: a folder created scoped to one project
// rendered under a different one. The sibling legacy tests cover the READ half
// (a HomeID-less row listed by every home); this one pins the MINT half, which
// had no coverage at all — that a create through a project's home mount stamps
// the home it was made in, so the row never depends on adoption to be scoped,
// and project B listing FIRST cannot claim it.
func TestRegression_HomeFolderCreatedInOneProjectAppearsInAnother(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	projectA := createProjectForHome(t, h)
	projectB := createProjectForHome(t, h)
	baseA := "/v0/projects/" + projectA + "/home"
	baseB := "/v0/projects/" + projectB + "/home"

	root := createChatFolder(t, h, baseA, "scoped-root", "")
	nested := createChatFolder(t, h, baseA, "scoped-nested", root.ID)
	h.Quiesce()

	// B reads before A ever does: nothing here may fall through to the
	// first-lister adoption the legacy path relies on.
	inB := listChatFolders(t, h, baseB)
	for _, made := range []string{root.ID, nested.ID} {
		_, leaked := chatFolderByID(inB, made)
		assert.False(t, leaked,
			"a folder created in project A's home must not be listed by project B: %s", made)
	}

	inA := listChatFolders(t, h, baseA)
	for _, made := range []string{root.ID, nested.ID} {
		_, kept := chatFolderByID(inA, made)
		assert.True(t, kept, "project A's home lists the folder it created: %s", made)
	}

	ctx := context.Background()
	for _, made := range []string{root.ID, nested.ID} {
		f, err := h.app.GORM.Folders.FindByKey(ctx, made)
		require.NoError(t, err)
		require.NotNil(t, f)
		assert.NotEmpty(t, f.HomeID,
			"the create itself stamps the home, without waiting for a read to adopt it: %s", made)
	}
}

// The other half of the same guarantee: a create must never be able to mint a
// home-scoped folder with no home at all. The repo mount takes its scope from
// :repoId, and an empty one there would produce exactly the HomeID-less row the
// legacy tests describe — a fresh one, on today's code.
func TestRegression_FolderCreateWithNoScopeMintsAHomelessRow(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	project := createProjectForHome(t, h)

	before := countFolders(t, h)
	h.postError("/v0/projects/"+project+"/repos//chats/folders",
		map[string]string{"name": "homeless", "parentId": ""}, http.StatusBadRequest)
	h.Quiesce()

	assert.Equal(t, before, countFolders(t, h),
		"a create with no repo and no home writes no folder row at all")
}

func countFolders(t *testing.T, h *harness) int {
	t.Helper()
	all, err := h.app.GORM.Folders.FindAll(context.Background())
	require.NoError(t, err)
	return len(all)
}
