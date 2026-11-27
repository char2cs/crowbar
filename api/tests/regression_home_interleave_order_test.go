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
	reposWS := watchEntities(t, h, stream.KindRepo)
	id := addRepo(t, h, reposWS, projectID, gitRepoWithCommit(t))
	h.Quiesce()
	return id
}

func TestRegression_RepoCannotBeReorderedPastAHomeChat(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	projectID := imported.projectID
	repoA := imported.repoID
	repoB := addSecondRepo(t, h, projectID)

	c1 := createHomeChat(t, h, projectID)
	c2 := createHomeChat(t, h, projectID)

	var placed placedChatDTO
	h.patch("/v0/projects/"+projectID+"/home/chats/"+c2+"/placement",
		map[string]any{"parentId": "", "order": 0}, &placed)
	h.Quiesce()

	chats := homeChatOrders(t, h, projectID)
	repos := repoOrders(t, h, projectID)
	t.Logf("after chat drag: c1=%d c2=%d A=%d B=%d", chats[c1].Order, chats[c2].Order, repos[repoA].Order, repos[repoB].Order)

	_ = h.raw(http.MethodPatch, "/v0/projects/"+projectID+"/repos/"+repoA, map[string]any{"order": 3}, http.StatusNoContent).Body.Close()
	h.Quiesce()

	chats = homeChatOrders(t, h, projectID)
	repos = repoOrders(t, h, projectID)
	t.Logf("after repo drag to end: c1=%d c2=%d A=%d B=%d", chats[c1].Order, chats[c2].Order, repos[repoA].Order, repos[repoB].Order)
	assert.Greater(t, repos[repoA].Order, chats[c1].Order, "repo A dragged to the end must sort after chat c1")
	assert.Greater(t, repos[repoA].Order, chats[c2].Order, "repo A dragged to the end must sort after chat c2")
	assert.Greater(t, repos[repoA].Order, repos[repoB].Order)

	_ = h.raw(http.MethodPatch, "/v0/projects/"+projectID+"/repos/"+repoA, map[string]any{"order": 0}, http.StatusNoContent).Body.Close()
	h.Quiesce()
	chats = homeChatOrders(t, h, projectID)
	repos = repoOrders(t, h, projectID)
	t.Logf("after repo drag to front: c1=%d c2=%d A=%d B=%d", chats[c1].Order, chats[c2].Order, repos[repoA].Order, repos[repoB].Order)
	assert.Less(t, repos[repoA].Order, chats[c1].Order, "repo A dragged to the front must sort before chat c1")
	assert.Less(t, repos[repoA].Order, chats[c2].Order, "repo A dragged to the front must sort before chat c2")
}

func TestRegression_HomeChatDragCountsInvisibleWorkspaceAnchors(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	projectID := imported.projectID
	ctx := context.Background()
	second := chatlessLockedBranch(t, h, imported, "release/anchor")

	c1 := createHomeChat(t, h, projectID)
	c2 := createHomeChat(t, h, projectID)
	c3 := createHomeChat(t, h, projectID)

	mainBefore, err := h.app.Repositories.Placement.GetNode(ctx, imported.workspaceID)
	require.NoError(t, err)
	secondBefore, err := h.app.Repositories.Placement.GetNode(ctx, second)
	require.NoError(t, err)

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

	mainAfter, err := h.app.Repositories.Placement.GetNode(ctx, imported.workspaceID)
	require.NoError(t, err)
	secondAfter, err := h.app.Repositories.Placement.GetNode(ctx, second)
	require.NoError(t, err)
	assert.Equal(t, mainBefore.Order, mainAfter.Order, "a home-level chat drag must not renumber a repo's locked branch")
	assert.Equal(t, secondBefore.Order, secondAfter.Order, "a home-level chat drag must not renumber a repo's locked branch")
}

func TestRegression_RepoFiledIntoHomeFolderLeavesTheRootDense(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	projectID := imported.projectID
	repoA := imported.repoID
	repoB := addSecondRepo(t, h, projectID)
	base := "/v0/projects/" + projectID + "/home"

	c1 := createHomeChat(t, h, projectID)
	c2 := createHomeChat(t, h, projectID)
	folder := createChatFolder(t, h, base, "F", "")

	for i, id := range []string{c1, c2} {
		h.patch(base+"/chats/"+id+"/placement", map[string]any{"parentId": "", "order": i * 2}, nil)
		h.Quiesce()
	}
	_ = h.raw(http.MethodPatch, "/v0/projects/"+projectID+"/repos/"+repoA, map[string]any{"order": 1}, http.StatusNoContent).Body.Close()
	h.Quiesce()
	_ = h.raw(http.MethodPatch, "/v0/projects/"+projectID+"/repos/"+repoB, map[string]any{"order": 3}, http.StatusNoContent).Body.Close()
	h.Quiesce()
	h.patch(base+"/chats/folders/"+folder.ID, map[string]any{"parentId": "", "order": 4}, nil)
	h.Quiesce()
	chats, repos := homeChatOrders(t, h, projectID), repoOrders(t, h, projectID)
	folders := listChatFolders(t, h, base)
	f, _ := chatFolderByID(folders, folder.ID)
	t.Logf("before: c1=%d A=%d c2=%d B=%d F=%d", chats[c1].Order, repos[repoA].Order, chats[c2].Order, repos[repoB].Order, f.Order)
	require.Equal(t, []int{0, 1, 2, 3, 4}, []int{chats[c1].Order, repos[repoA].Order, chats[c2].Order, repos[repoB].Order, f.Order})

	_ = h.raw(http.MethodPatch, "/v0/projects/"+projectID+"/repos/"+repoA,
		map[string]any{"folderId": folder.ID, "order": 0}, http.StatusNoContent).Body.Close()
	h.Quiesce()

	chats, repos = homeChatOrders(t, h, projectID), repoOrders(t, h, projectID)
	f, _ = chatFolderByID(listChatFolders(t, h, base), folder.ID)
	t.Logf("after: c1=%d c2=%d B=%d F=%d (A in folder: %q)", chats[c1].Order, chats[c2].Order, repos[repoB].Order, f.Order, repos[repoA].FolderID)
	assert.Equal(t, folder.ID, repos[repoA].FolderID)
	assert.Equal(t, []int{0, 1, 2, 3}, []int{chats[c1].Order, chats[c2].Order, repos[repoB].Order, f.Order},
		"the level repoA left must stay dense in its drawn order [c1, c2, repoB, F]")
}
