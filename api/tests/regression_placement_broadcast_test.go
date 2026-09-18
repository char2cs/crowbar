//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A top-level placement renumbers OTHER repo header rows server-side, but
// only ever announced the dragged subject — the repo path one RepoDTO, the
// chat path folders and the chat — so a live sidebar kept stale repo orders
// (tied against the moved row's) until a reload.

func TestRegression_RepoReorderBroadcastsEveryShiftedRepo(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	repoA := imported.repoID
	repoB := addSecondRepo(t, h, p)

	reposWS := h.dial("/v0/projects/" + p + "/repos")
	// [A, B] -> drag B to the front: A shifts to 1 as collateral.
	h.raw(http.MethodPatch, "/v0/projects/"+p+"/repos/"+repoB, map[string]any{"order": 0}, http.StatusNoContent).Body.Close()

	// The socket opens on a snapshot carrying the PRE-drag orders, so read
	// frames until both repos have been announced at their decided ones.
	seen := map[string]float64{}
	readUntil(t, reposWS, func(m map[string]any) bool {
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

	reposWS := h.dial("/v0/projects/" + p + "/repos")
	// [repo, c1] -> drag c1 to the front: the repo shifts to 1.
	h.patch("/v0/projects/"+p+"/home/chats/"+c1+"/placement", map[string]any{"parentId": "", "order": 0}, nil)

	frame := readUntil(t, reposWS, func(m map[string]any) bool {
		id, _ := m["id"].(string)
		order, _ := m["order"].(float64)
		return id == imported.repoID && order == 1
	})
	require.Equal(t, float64(1), frame["order"], "the repo header shifted by a chat drag must be announced")
}
