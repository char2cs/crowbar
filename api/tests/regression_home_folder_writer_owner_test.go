//go:build integration

package tests

import (
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// homeTopLevel is topLevel plus the project's root home folders, on the one
// order the sidebar sorts all three kinds by.
func homeTopLevel(t *testing.T, h *harness, projectID, homeOwnerID string) []topRow {
	t.Helper()
	rows := topLevel(t, h, projectID, homeOwnerID)
	var folders []struct {
		ID       string `json:"id"`
		ParentID string `json:"parentId"`
		Order    int    `json:"order"`
	}
	h.get("/v0/projects/"+projectID+"/home/chats/folders", &folders)
	for _, f := range folders {
		if f.ParentID == "" {
			rows = append(rows, topRow{id: f.ID, kind: "folder", order: f.Order})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].order < rows[j].order })
	return rows
}

// The home FOLDER writer (tree Create/Move via globalSnapshotIn/
// globalSnapshotAround) counts the project-home owning chat as a root
// sibling; the chat writer (homeSnapshotAround -> withoutHomeOwner), the repo
// writer and the sidebar all exclude it. A folder create renumbers the hidden
// owner into the level and every folder drop past its slot lands one early.
func TestRegression_HomeFolderPlacementIgnoresHiddenOwner(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	home := "/v0/projects/" + p + "/home"
	var homeWS struct {
		OwningChatID string `json:"owningChatId"`
	}
	h.get(home, &homeWS)
	c1 := createHomeChatStub(t, h, p)
	c2 := createHomeChatStub(t, h, p)

	folder := createChatFolder(t, h, home, "hf", "")
	h.Quiesce()
	rows := homeTopLevel(t, h, p, homeWS.OwningChatID)
	assertDense(t, rows)
	require.Equal(t, []string{imported.repoID, c1, c2, folder.ID}, idsOf(rows))

	// rendered rest without the folder = [repo, c1, c2]; "after c1" is index 2
	h.raw(http.MethodPatch, home+"/chats/folders/"+folder.ID, map[string]any{"parentId": "", "order": 2}, http.StatusOK).Body.Close()
	h.Quiesce()
	rows = homeTopLevel(t, h, p, homeWS.OwningChatID)
	assertDense(t, rows)
	assert.Equal(t, []string{imported.repoID, c1, folder.ID, c2}, idsOf(rows))

	// "tail" over rest [repo, c1, c2] is index 3
	h.raw(http.MethodPatch, home+"/chats/folders/"+folder.ID, map[string]any{"parentId": "", "order": 3}, http.StatusOK).Body.Close()
	h.Quiesce()
	rows = homeTopLevel(t, h, p, homeWS.OwningChatID)
	assertDense(t, rows)
	assert.Equal(t, []string{imported.repoID, c1, c2, folder.ID}, idsOf(rows))

	// the owner is not a row of this level and must not have been counted into it
	owner := homeChatOrders(t, h, p)[homeWS.OwningChatID]
	assert.Equal(t, 0, owner.Order, "the hidden owner was renumbered as a sibling")
}
