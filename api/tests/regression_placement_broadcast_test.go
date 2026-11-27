//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

func TestRegression_RepoReorderBroadcastsEveryShiftedRepo(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	repoA := imported.repoID
	repoB := addSecondRepo(t, h, p)

	reposWS := watchEntities(t, h, stream.KindRepo)
	h.raw(http.MethodPatch, "/v0/projects/"+p+"/repos/"+repoB, map[string]any{"order": 0}, http.StatusNoContent).Body.Close()

	seen := map[string]float64{}
	reposWS.read(t, func(m map[string]any) bool {
		id, _ := m["id"].(string)
		if id == repoA || id == repoB {
			seen[id], _ = m["order"].(float64)
		}
		return seen[repoB] == 0 && seen[repoA] == 1 && len(seen) == 2
	})
	assert.Equal(t, float64(0), seen[repoB], "the subject's frame carries its decided order")
	assert.Equal(t, float64(1), seen[repoA], "the repo shifted as collateral is announced with its new order")
}

func TestRegression_HomeChatPlacementBroadcastsShiftedRepos(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	c1 := createHomeChatStub(t, h, p)

	reposWS := watchEntities(t, h, stream.KindRepo)
	h.patch("/v0/projects/"+p+"/home/chats/"+c1+"/placement", map[string]any{"parentId": "", "order": 0}, nil)

	frame := reposWS.read(t, func(m map[string]any) bool {
		id, _ := m["id"].(string)
		order, _ := m["order"].(float64)
		return id == imported.repoID && order == 1
	})
	require.Equal(t, float64(1), frame["order"], "the repo header shifted by a chat drag must be announced")
}
