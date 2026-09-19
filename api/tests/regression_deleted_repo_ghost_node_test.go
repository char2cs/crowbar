//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DELETE .../repos/:r drops the Repository row and its workspaces but never
// Forgets the repo's own Node row, so its header keeps a slot in whatever
// home container it was filed in. The sidebar no longer draws it, but the
// chat writer still counts it (mergeHomeNode appends every repo phantom found
// under a non-root parent, with no membership check), so a drop index the
// sidebar computes over the visible rows lands one slot off inside that
// folder.
func TestRegression_DeletedRepoLeavesAGhostNodeInItsHomeFolder(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	base := "/v0/projects/" + p + "/home"

	folder := createChatFolder(t, h, base, "F", "")
	h.Quiesce()
	// File the repo INTO the folder at slot 0, then delete the repo.
	h.raw(http.MethodPatch, "/v0/projects/"+p+"/repos/"+imported.repoID,
		map[string]any{"folderId": folder.ID, "order": 0}, http.StatusNoContent).Body.Close()
	h.Quiesce()
	reposWS := h.dial("/v0/projects/" + p + "/repos")
	h.raw(http.MethodDelete, "/v0/projects/"+p+"/repos/"+imported.repoID, nil, http.StatusAccepted).Body.Close()
	// The delete runs off the request path; its tombstone frame is the signal.
	readUntil(t, reposWS, func(m map[string]any) bool {
		return m["id"] == imported.repoID && m["status"] == "deleted"
	})
	h.QuiesceReactors()

	_, err := h.app.Repositories.Node.GetNode(context.Background(), imported.repoID)
	t.Logf("deleted repo's Node row after delete: err=%v", err)
	assert.Error(t, err, "a deleted repo must not keep a Node row")

	// Two chats filed into the folder, one at the front and one at the tail,
	// so the ghost sits between them: rendered [c1, c2].
	c1 := createHomeChatStub(t, h, p)
	c2 := createHomeChatStub(t, h, p)
	c3 := createHomeChatStub(t, h, p)
	h.patch(base+"/chats/"+c1+"/placement", map[string]any{"parentId": folder.ID, "order": 0}, nil)
	h.Quiesce()
	h.patch(base+"/chats/"+c2+"/placement", map[string]any{"parentId": folder.ID, "order": 5}, nil)
	h.Quiesce()

	// Drop c3 AFTER c2 (the end of the folder): the sidebar's index over the
	// visible rows is 2.
	h.patch(base+"/chats/"+c3+"/placement", map[string]any{"parentId": folder.ID, "order": 2}, nil)
	h.Quiesce()

	var chats []struct {
		ID       string `json:"id"`
		ParentID string `json:"parentId"`
		Order    int    `json:"order"`
	}
	h.get(base+"/chats", &chats)
	got := map[string]int{}
	for _, c := range chats {
		if c.ParentID == folder.ID {
			got[c.ID] = c.Order
		}
	}
	t.Logf("folder members: c1=%d c2=%d c3=%d", got[c1], got[c2], got[c3])
	require.Len(t, got, 3)
	assert.Equal(t, 0, got[c1], "c1 keeps slot 0")
	assert.Equal(t, 1, got[c2], "c2 keeps slot 1")
	assert.Equal(t, 2, got[c3], "c3 lands at the end, after c2")
}
