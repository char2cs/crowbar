//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every repo imported before the Node migration (every real pre-existing
// production repo — the migration ships with no backfill) has no Node row.
// The sidebar still draws it as a top-level row at order 0, and the drop
// planners count it, so the index a drag posts includes it. Both backend
// writers build the home level from Node.ListByParent alone, so a Node-less
// repo is simply not a sibling: a chat dropped BETWEEN two such repos lands
// after both of them.
func TestRegression_HomeChatDroppedBetweenTwoLegacyNodelessRepos(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	repoA := imported.repoID
	repoB := addSecondRepo(t, h, p)

	var home struct {
		OwningChatID string `json:"owningChatId"`
	}
	h.get("/v0/projects/"+p+"/home", &home)
	c1 := createHomeChatStub(t, h, p)
	c2 := createHomeChatStub(t, h, p)

	// The production shape: neither repo has a Node row.
	ctx := context.Background()
	require.NoError(t, h.app.Repositories.Node.Forget(ctx, repoA))
	require.NoError(t, h.app.Repositories.Node.Forget(ctx, repoB))
	// And the two home chats tie the repos at 0, as every never-dragged
	// legacy level does.
	require.NoError(t, h.app.Repositories.Node.SetOrder(ctx, c1, 0))
	require.NoError(t, h.app.Repositories.Node.SetOrder(ctx, c2, 0))
	h.Quiesce()

	before := topLevel(t, h, p, home.OwningChatID)
	t.Logf("before: %+v", before)
	require.Len(t, before, 4)
	// The sidebar's tie order: chats first, then repo headers (tree.Rank).
	// Displayed [c1, c2, repoA, repoB]. Drag c1 BETWEEN the two repos.
	h.patch("/v0/projects/"+p+"/home/chats/"+c1+"/placement", map[string]any{"parentId": "", "order": 2}, nil)
	h.Quiesce()

	after := topLevel(t, h, p, home.OwningChatID)
	t.Logf("after c1->2: %+v", after)
	assertDense(t, after)
	// Rank-agnostic: the repos may tie among themselves, but c1 must sit
	// between exactly one repo and the other.
	idx := map[string]int{}
	for i, r := range after {
		idx[r.id] = i
	}
	assert.Equal(t, 2, idx[c1], "c1 must land at the slot the drop line was drawn on")
	assert.Equal(t, 0, idx[c2])
	assert.True(t, (idx[repoA] == 1 && idx[repoB] == 3) || (idx[repoB] == 1 && idx[repoA] == 3),
		"exactly one repo must sit before c1 and the other after it")
}

// The repo writer has the same blind spot from the other side: dragging a
// Node-less repo past a home chat counts only the Node-backed siblings, so
// the OTHER Node-less repo keeps its implicit 0 and the requested slot is
// off by one.
func TestRegression_LegacyNodelessRepoDraggedPastHomeChatLandsWhereIndicated(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	repoA := imported.repoID
	repoB := addSecondRepo(t, h, p)

	var home struct {
		OwningChatID string `json:"owningChatId"`
	}
	h.get("/v0/projects/"+p+"/home", &home)
	c1 := createHomeChatStub(t, h, p)

	ctx := context.Background()
	require.NoError(t, h.app.Repositories.Node.Forget(ctx, repoA))
	require.NoError(t, h.app.Repositories.Node.Forget(ctx, repoB))
	require.NoError(t, h.app.Repositories.Node.SetOrder(ctx, c1, 0))
	h.Quiesce()

	before := topLevel(t, h, p, home.OwningChatID)
	t.Logf("before: %+v", before)
	require.Len(t, before, 3)
	// Displayed [c1, repoA, repoB] (ties: chats then repos). Drag repoB to
	// the very top (index 0).
	h.raw(http.MethodPatch, "/v0/projects/"+p+"/repos/"+repoB, map[string]any{"order": 0}, http.StatusNoContent).Body.Close()
	h.Quiesce()

	after := topLevel(t, h, p, home.OwningChatID)
	t.Logf("after repoB->0: %+v", after)
	assertDense(t, after)
	assert.Equal(t, []string{repoB, c1, repoA}, idsOf(after))
}
