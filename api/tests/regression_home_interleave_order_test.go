//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A project's top level is ONE interleaved list of repo header rows and home
// chats/folders, sorted on one `order` (rows-from-home.ts). These tests drive
// the two write paths that list is reordered through — PATCH .../repos/:r
// {order} and PATCH .../home/chats/:id/placement {order} — and read back the
// orders both GETs serve, exactly as the sidebar does.

type homeRepoOrder struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FolderID string `json:"folderId"`
	Order    int    `json:"order"`
}

func repoOrders(
	t *testing.T,
	h *harness,
	projectID string,
) map[string]homeRepoOrder {
	t.Helper()
	var rows []homeRepoOrder
	h.get("/v0/projects/"+projectID+"/repos", &rows)
	out := map[string]homeRepoOrder{}
	for _, r := range rows {
		out[r.ID] = r
	}
	return out
}

func homeChatOrders(
	t *testing.T,
	h *harness,
	projectID string,
) map[string]agentChatDTO {
	t.Helper()
	var rows []agentChatDTO
	h.get("/v0/projects/"+projectID+"/home/chats", &rows)
	out := map[string]agentChatDTO{}
	for _, c := range rows {
		out[c.ID] = c
	}
	return out
}

func createHomeChat(
	t *testing.T,
	h *harness,
	projectID string,
) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post("/v0/projects/"+projectID+"/home/chats", map[string]string{"provider": "livestub"},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.Quiesce()
	return created.ID
}

func addSecondRepo(
	t *testing.T,
	h *harness,
	projectID string,
) string {
	t.Helper()
	reposWS := h.dial("/v0/projects/" + projectID + "/repos")
	id := addRepo(t, h, reposWS, projectID, gitRepoWithCommit(t))
	h.Quiesce()
	return id
}

// TestRegression_RepoCannotBeReorderedPastAHomeChat is K3's repo half. A home
// chat created at the panel root (POST .../home/chats, CreateChat's
// parentID=="" branch -> SpawnChat) never gets a Node row — only a placement
// through the tree package mints one — while placeRepoAmongHomeSiblings
// (usecases/project/project.go) builds the repo's sibling space from
// Nodes.ListByParent("") alone. The chat is invisible to the repo's drag, its
// own Chat.Order is never shifted, and the repo can never be placed past it.
func TestRegression_RepoCannotBeReorderedPastAHomeChat(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	projectID := imported.projectID
	repoA := imported.repoID
	repoB := addSecondRepo(t, h, projectID)

	c1 := createHomeChat(t, h, projectID)
	c2 := createHomeChat(t, h, projectID)

	// Give every visible row a real, distinct order through the CHAT path
	// first (c2 to the front), so the assertion below is never decided by a
	// tie the sidebar breaks arbitrarily.
	var placed placedChatDTO
	h.patch("/v0/projects/"+projectID+"/home/chats/"+c2+"/placement",
		map[string]any{"parentId": "", "order": 0}, &placed)
	h.Quiesce()

	chats := homeChatOrders(t, h, projectID)
	repos := repoOrders(t, h, projectID)
	t.Logf("after chat drag: c1=%d c2=%d A=%d B=%d", chats[c1].Order, chats[c2].Order, repos[repoA].Order, repos[repoB].Order)

	// The user's gesture: drag repo A to the END of the top level, after both
	// chats. The frontend sends the visible target index.
	_ = h.raw(http.MethodPatch, "/v0/projects/"+projectID+"/repos/"+repoA, map[string]any{"order": 3}, http.StatusNoContent).Body.Close()
	h.Quiesce()

	chats = homeChatOrders(t, h, projectID)
	repos = repoOrders(t, h, projectID)
	t.Logf("after repo drag to end: c1=%d c2=%d A=%d B=%d", chats[c1].Order, chats[c2].Order, repos[repoA].Order, repos[repoB].Order)
	assert.Greater(t, repos[repoA].Order, chats[c1].Order, "repo A dragged to the end must sort after chat c1")
	assert.Greater(t, repos[repoA].Order, chats[c2].Order, "repo A dragged to the end must sort after chat c2")
	assert.Greater(t, repos[repoA].Order, repos[repoB].Order)

	// And back to the front, before both chats.
	_ = h.raw(http.MethodPatch, "/v0/projects/"+projectID+"/repos/"+repoA, map[string]any{"order": 0}, http.StatusNoContent).Body.Close()
	h.Quiesce()
	chats = homeChatOrders(t, h, projectID)
	repos = repoOrders(t, h, projectID)
	t.Logf("after repo drag to front: c1=%d c2=%d A=%d B=%d", chats[c1].Order, chats[c2].Order, repos[repoA].Order, repos[repoB].Order)
	assert.Less(t, repos[repoA].Order, chats[c1].Order, "repo A dragged to the front must sort before chat c1")
	assert.Less(t, repos[repoA].Order, chats[c2].Order, "repo A dragged to the front must sort before chat c2")
}

// TestRegression_HomeChatDragCountsInvisibleWorkspaceAnchors is K3's chat
// half. Every workspace's Node{Kind:workspace} row is minted at parent ""
// (hierarchy/node.go, owning_chats.go) — the SAME bare root project home's
// chats and repo headers share — and homeSnapshotAround/mergeHomeNode folds
// every locked branch's anchor into the home level's densify. Those rows are
// drawn INSIDE their repo, never at the top level, so the visible index the
// sidebar asks for lands elsewhere, and the repo-internal order of the locked
// branches is rewritten as collateral.
func TestRegression_HomeChatDragCountsInvisibleWorkspaceAnchors(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h) // repo with a locked managed "main": one anchor at ""
	projectID := imported.projectID
	ctx := context.Background()
	second := chatlessLockedBranch(t, h, imported, "release/anchor") // a second anchor at ""

	c1 := createHomeChat(t, h, projectID)
	c2 := createHomeChat(t, h, projectID)
	c3 := createHomeChat(t, h, projectID)

	mainBefore, err := h.app.Repositories.Node.GetNode(ctx, imported.workspaceID)
	require.NoError(t, err)
	secondBefore, err := h.app.Repositories.Node.GetNode(ctx, second)
	require.NoError(t, err)

	// Visible top level: [repo, c1, c2, c3]. Drag c3 to the END (index 3).
	var placed placedChatDTO
	h.patch("/v0/projects/"+projectID+"/home/chats/"+c3+"/placement",
		map[string]any{"parentId": "", "order": 3}, &placed)
	h.Quiesce()

	chats := homeChatOrders(t, h, projectID)
	repos := repoOrders(t, h, projectID)
	t.Logf("c1=%d c2=%d c3=%d repo=%d", chats[c1].Order, chats[c2].Order, chats[c3].Order, repos[imported.repoID].Order)
	assert.Greater(t, chats[c3].Order, chats[c1].Order, "c3 dragged to the end must sort after c1")
	assert.Greater(t, chats[c3].Order, chats[c2].Order, "c3 dragged to the end must sort after c2")
	assert.Greater(t, chats[c3].Order, repos[imported.repoID].Order, "c3 dragged to the end must sort after the repo header")

	mainAfter, err := h.app.Repositories.Node.GetNode(ctx, imported.workspaceID)
	require.NoError(t, err)
	secondAfter, err := h.app.Repositories.Node.GetNode(ctx, second)
	require.NoError(t, err)
	assert.Equal(t, mainBefore.Order, mainAfter.Order, "a home-level chat drag must not renumber a repo's locked branch")
	assert.Equal(t, secondBefore.Order, secondAfter.Order, "a home-level chat drag must not renumber a repo's locked branch")
}
