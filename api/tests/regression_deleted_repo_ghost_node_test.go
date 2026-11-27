//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

func TestRegression_DeletedRepoLeavesAGhostNodeInItsHomeFolder(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	base := "/v0/projects/" + p + "/home"

	folder := createChatFolder(t, h, base, "F", "")
	h.Quiesce()
	h.raw(http.MethodPatch, "/v0/projects/"+p+"/repos/"+imported.repoID,
		map[string]any{"folderId": folder.ID, "order": 0}, http.StatusNoContent).Body.Close()
	h.Quiesce()
	reposWS := watchEntities(t, h, stream.KindRepo)
	h.raw(http.MethodDelete, "/v0/projects/"+p+"/repos/"+imported.repoID, nil, http.StatusAccepted).Body.Close()
	reposWS.read(t, func(m map[string]any) bool {
		return m["id"] == imported.repoID && m["gone"] == true
	})
	h.QuiesceReactors()

	_, err := h.app.Repositories.Placement.GetNode(context.Background(), imported.repoID)
	t.Logf("deleted repo's Node row after delete: err=%v", err)
	assert.Error(t, err, "a deleted repo must not keep a Node row")

	c1 := createHomeChatStub(t, h, p)
	c2 := createHomeChatStub(t, h, p)
	c3 := createHomeChatStub(t, h, p)
	h.patch(base+"/chats/"+c1+"/placement", map[string]any{"parentId": folder.ID, "order": 0}, nil)
	h.Quiesce()
	h.patch(base+"/chats/"+c2+"/placement", map[string]any{"parentId": folder.ID, "order": 5}, nil)
	h.Quiesce()

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
