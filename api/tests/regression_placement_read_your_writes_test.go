//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// A placement's Node writes rode the async Send path, so the sidebar's own
// re-read right after the PATCH (refreshRepoPlacements, the home reseed)
// could serve the PRE-write order and overwrite the decided one the response
// and broadcast had already delivered: the row landed, then snapped back.
func TestRegression_PlacementReadBackRightAfterThePatchIsTheDecidedOrder(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	c1 := createHomeChatStub(t, h, p)
	c2 := createHomeChatStub(t, h, p)
	h.Quiesce()

	var repos []struct {
		ID    string `json:"id"`
		Order int    `json:"order"`
	}
	orderOfRepo := func() int {
		h.get("/v0/projects/"+p+"/repos", &repos)
		for _, r := range repos {
			if r.ID == imported.repoID {
				return r.Order
			}
		}
		t.Fatal("repo missing")
		return -1
	}
	for i := range 40 {
		want := i % 3
		h.raw(http.MethodPatch, "/v0/projects/"+p+"/repos/"+imported.repoID,
			map[string]any{"folderId": "", "order": want}, http.StatusNoContent).Body.Close()
		require.Equalf(t, want, orderOfRepo(), "read-back %d right after the PATCH must be the decided order", i)
	}

	orderOfChat := func(id string) int {
		return homeChatOrders(t, h, p)[id].Order
	}
	for i := range 40 {
		want := i % 3
		h.patch("/v0/projects/"+p+"/home/chats/"+c1+"/placement", map[string]any{"parentId": "", "order": want}, nil)
		require.Equalf(t, want, orderOfChat(c1), "chat read-back %d right after the PATCH must be the decided order", i)
	}
	_ = c2
}
