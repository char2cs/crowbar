//go:build integration

package tests

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type agentChatFolderDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	ParentID    string `json:"parentId"`
	Title       string `json:"title"`
	Order       int    `json:"order"`
}

type chatFolderMutation struct {
	Folder  agentChatFolderDTO   `json:"folder"`
	Shifted []agentChatFolderDTO `json:"shifted"`
}

type placedChatDTO struct {
	Chat struct {
		ID       string `json:"id"`
		ParentID string `json:"parentId"`
		Order    int    `json:"order"`
	} `json:"chat"`
	Shifted []agentChatFolderDTO `json:"shifted"`
}

func createChatFolder(
	t *testing.T,
	h *harness,
	base string,
	name string,
	parentID string,
) agentChatFolderDTO {
	t.Helper()
	var out chatFolderMutation
	h.post(base+"/chats/folders",
		map[string]string{"name": name, "parentId": parentID},
		http.StatusCreated, &out)
	require.NotEmpty(t, out.Folder.ID, "a folder create must answer with the row it made")
	return out.Folder
}

func listChatFolders(
	t *testing.T,
	h *harness,
	base string,
) []agentChatFolderDTO {
	t.Helper()
	var folders []agentChatFolderDTO
	h.get(base+"/chats/folders", &folders)
	return folders
}

func placeChat(
	t *testing.T,
	h *harness,
	base string,
	chatID string,
	body map[string]any,
) placedChatDTO {
	t.Helper()
	var out placedChatDTO
	h.patch(base+"/chats/"+chatID+"/placement", body, &out)
	return out
}

func chatFolderByID(
	rows []agentChatFolderDTO,
	id string,
) (agentChatFolderDTO, bool) {
	for _, row := range rows {
		if row.ID == id {
			return row, true
		}
	}
	return agentChatFolderDTO{}, false
}

func chatIDs(
	t *testing.T,
	h *harness,
	base string,
) []string {
	t.Helper()
	h.Quiesce()
	var list []agentChatDTO
	h.get(base+"/chats", &list)
	list = conversationsOnly(list)
	out := make([]string, 0, len(list))
	for _, c := range list {
		out = append(out, c.ID)
	}
	return out
}

func chatOrderOf(
	t *testing.T,
	h *harness,
	base string,
	chatID string,
) int {
	t.Helper()
	h.Quiesce()
	var list []agentChatDTO
	h.get(base+"/chats", &list)
	for _, row := range list {
		if row.ID == chatID {
			return row.Order
		}
	}
	t.Fatalf("chat %s is not in the list at %s", chatID, base)
	return 0
}

func TestRegression_ChatDeleteCascadesToItsThreads(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)

	root := createAgentChat(t, h, ws)
	child := createAgentChat(t, h, ws)
	grandchild := createAgentChat(t, h, ws)
	bystander := createAgentChat(t, h, ws)

	placeChat(t, h, base, child, map[string]any{"parentId": root})
	h.Quiesce()
	placeChat(t, h, base, grandchild, map[string]any{"parentId": child})
	h.Quiesce()

	inside := createChatFolder(t, h, base, "spikes", root)

	resp := h.raw(http.MethodDelete, base+"/chats/"+root, nil, http.StatusOK)
	_ = resp.Body.Close()
	h.Quiesce()

	remaining := chatIDs(t, h, base)
	assert.Equal(t, []string{bystander}, remaining,
		"a deleted chat takes every chat threaded below it, and nothing else")

	_, found := chatFolderByID(listChatFolders(t, h, base), inside.ID)
	assert.False(t, found, "a folder caught inside the deleted subtree goes with it")

	getResp := h.raw(http.MethodGet, base+"/chats/"+grandchild, nil, http.StatusNotFound)
	_ = getResp.Body.Close()
}

func TestRegression_ChatFolderDeletePromotesItsChildren(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)

	outer := createChatFolder(t, h, base, "outer", "")
	inner := createChatFolder(t, h, base, "inner", outer.ID)
	chat := createAgentChat(t, h, ws)
	placeChat(t, h, base, chat, map[string]any{"parentId": outer.ID})
	h.Quiesce()

	var deleted struct {
		Shifted []agentChatFolderDTO `json:"shifted"`
	}
	h.del(base+"/chats/folders/"+outer.ID, nil, http.StatusOK, &deleted)
	h.Quiesce()

	rows := listChatFolders(t, h, base)
	_, gone := chatFolderByID(rows, outer.ID)
	assert.False(t, gone, "the folder itself is removed")
	promoted, ok := chatFolderByID(rows, inner.ID)
	require.True(t, ok, "the child folder survives its parent")
	assert.Equal(t, "", promoted.ParentID, "and rises to the folder's own parent")

	var listed []agentChatDTO
	h.get(base+"/chats", &listed)
	list := conversationsOnly(listed)
	require.Len(t, list, 1)
	assert.Equal(t, chat, list[0].ID, "the chat outlives the folder that held it")
	assert.Equal(t, "", list[0].ParentID, "and is promoted, never deleted")
}

func TestRegression_ChatTreeMoveRefusedWhenItWouldCycle(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)

	outer := createChatFolder(t, h, base, "outer", "")
	inner := createChatFolder(t, h, base, "inner", outer.ID)

	msg := h.mutationError(http.MethodPatch, base+"/chats/folders/"+outer.ID,
		map[string]string{"parentId": inner.ID}, http.StatusConflict)
	assert.Contains(t, msg, "inside itself",
		"the refusal has to say what is wrong, not just refuse")

	h.mutationError(http.MethodPatch, base+"/chats/folders/"+outer.ID,
		map[string]string{"parentId": outer.ID}, http.StatusConflict)

	parent := createAgentChat(t, h, ws)
	thread := createAgentChat(t, h, ws)
	placeChat(t, h, base, thread, map[string]any{"parentId": parent})
	h.Quiesce()

	h.mutationError(http.MethodPatch, base+"/chats/"+parent+"/placement",
		map[string]string{"parentId": thread}, http.StatusConflict)

	rows := listChatFolders(t, h, base)
	outerRow, ok := chatFolderByID(rows, outer.ID)
	require.True(t, ok)
	assert.Equal(t, "", outerRow.ParentID)
	innerRow, ok := chatFolderByID(rows, inner.ID)
	require.True(t, ok)
	assert.Equal(t, outer.ID, innerRow.ParentID)

	var reread agentChatDetail
	h.get(base+"/chats/"+parent, &reread)
	assert.Equal(t, "", reread.ParentID, "a refused placement must never rewrite lineage")
}

func TestRegression_FolderRenameSucceedsWithAWorkingChatFiledUnderIt(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	sprint := createChatFolder(t, h, base, "sprint", "")
	chat := createAgentChat(t, h, imported)
	placeChat(t, h, base, chat, map[string]any{"parentId": sprint.ID})
	postHook(t, h, base, "livestub", startChat(t, h, base, chat), "user_prompt", `{"prompt":"still going"}`)
	h.Quiesce()

	deleteMsg := h.mutationError(http.MethodDelete, base+"/chats/folders/"+sprint.ID, nil, http.StatusConflict)
	assert.Contains(t, deleteMsg, "working", "the delete guard must still refuse this same working subtree")

	var renamed chatFolderMutation
	h.patch(base+"/chats/folders/"+sprint.ID, map[string]string{"name": "renamed-while-working"}, &renamed)
	assert.Equal(t, "renamed-while-working", renamed.Folder.Title)

	rows := listChatFolders(t, h, base)
	row, ok := chatFolderByID(rows, sprint.ID)
	require.True(t, ok)
	assert.Equal(t, "renamed-while-working", row.Title, "the rename actually persisted")
}

func TestRegression_ChatTreeRefusesCrossWorkspaceParentage(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	a := importWritableWorkspace(t, h)
	b := importWritableWorkspace(t, h)

	foreignFolder := createChatFolder(t, h, repoBase(b), "elsewhere", "")
	foreignChat := createAgentChat(t, h, b)

	msg := h.mutationError(http.MethodPost, repoBase(a)+"/chats/folders",
		map[string]string{"name": "spikes", "parentId": foreignFolder.ID}, http.StatusConflict)
	assert.Contains(t, msg, "repo")

	msg = h.mutationError(http.MethodPost, repoBase(a)+"/chats/folders",
		map[string]string{"name": "spikes-2", "parentId": foreignChat}, http.StatusConflict)
	assert.Contains(t, msg, "repo")

	own := createAgentChat(t, h, a)
	msg = h.mutationError(http.MethodPatch, repoBase(a)+"/chats/"+own+"/placement",
		map[string]string{"parentId": foreignChat}, http.StatusConflict)
	assert.Contains(t, msg, "workspace")

	h.patch(repoBase(a)+"/chats/folders/"+foreignFolder.ID, map[string]any{"name": "renamed"}, nil)

	placed := placeChat(t, h, repoBase(a), foreignChat, map[string]any{"parentId": foreignFolder.ID})
	assert.Equal(t, foreignFolder.ID, placed.Chat.ParentID,
		"the reparent, addressed through the other repo's mount, actually landed")

	var reread agentChatDetail
	h.get(repoBase(b)+"/chats/"+foreignChat, &reread)
	assert.Equal(t, foreignFolder.ID, reread.ParentID,
		"and is visible back through the chat's own repo mount too")
}

func TestRegression_ChatTreeOrderIsDenseAndReturnsWhatItShifted(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	ws := importProjectHomeHoldsDefault(t, h)
	base := repoBase(ws)

	chat := createChatWithProvider(t, h, base, "livestub", ws.workspaceID, "")
	chatOrder := chatOrderOf(t, h, base, chat)

	first := createChatFolder(t, h, base, "a", "")
	require.Greater(t, first.Order, chatOrder,
		"a new folder lands after the chat already at that level")

	second := createChatFolder(t, h, base, "b", "")
	require.Equal(t, first.Order+1, second.Order, "and the next one lands after that")

	var moved chatFolderMutation
	h.patch(base+"/chats/folders/"+second.ID, map[string]any{"order": 0}, &moved)
	assert.Equal(t, 0, moved.Folder.Order)
	shifted, ok := chatFolderByID(moved.Shifted, first.ID)
	require.True(t, ok, "the folder the drop pushed down must ride back with the answer")
	assert.Equal(t, first.Order+1, shifted.Order,
		"the drop pushed it down exactly one slot")
	assert.NotContains(t, folderIDs(moved.Shifted), second.ID, "the subject is not its own collateral")

	h.Quiesce()
	var reread agentChatDetail
	h.get(base+"/chats/"+chat, &reread)
	assert.Greater(t, reread.Order, chatOrder,
		"the drop landed above the chat, so the renumber pushed the chat down")

	assertDenseChatLevel(t, h, base, "", ws.workspaceID)

	placed := placeChat(t, h, base, chat, map[string]any{"order": 0})
	assert.Equal(t, 0, placed.Chat.Order)
	assert.NotEmpty(t, placed.Shifted, "a chat drop reports the folders it renumbered")
	assertDenseChatLevel(t, h, base, "", ws.workspaceID)
}

func TestRegression_ChatFoldersWorkOnHomeWorkspace(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := "/v0/projects/" + imported.projectID + "/home"

	var chat struct {
		ID string `json:"id"`
	}
	h.post(base+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &chat)
	require.NotEmpty(t, chat.ID)
	h.Quiesce()

	folder := createChatFolder(t, h, base, "spikes", "")
	rows := listChatFolders(t, h, base)
	require.Len(t, rows, 1)
	assert.Equal(t, "spikes", rows[0].Title)

	placed := placeChat(t, h, base, chat.ID, map[string]any{"parentId": folder.ID})
	assert.Equal(t, folder.ID, placed.Chat.ParentID, "a home chat files into a home folder")

	h.patch(base+"/chats/folders/"+folder.ID, map[string]any{"name": "experiments"}, nil)
	renamed := listChatFolders(t, h, base)
	require.Len(t, renamed, 1)
	assert.Equal(t, "experiments", renamed[0].Title)

	h.del(base+"/chats/folders/"+folder.ID, nil, http.StatusOK, nil)
	assert.Empty(t, listChatFolders(t, h, base))

	h.Quiesce()
	var list []agentChatDTO
	h.get(base+"/chats", &list)
	var found bool
	for _, row := range conversationsOnly(list) {
		if row.ID == chat.ID {
			found = true
		}
	}
	assert.True(t, found, "the home chat outlived the folder it was filed in")
}

func TestRegression_ChatFolderMutationsReachTheProjectsScope(t *testing.T) {
	h := newHarness(t)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)
	feed := recordSidebar(t, h)

	folder := createChatFolder(t, h, base, "spikes", "")
	awaitFolder(feed, folder.ID, "spikes")

	h.patch(base+"/chats/folders/"+folder.ID, map[string]any{"name": "experiments"}, nil)
	awaitFolder(feed, folder.ID, "experiments")

	h.del(base+"/chats/folders/"+folder.ID, nil, http.StatusOK, nil)
	feed.awaitGone(stream.KindFolder, folder.ID)
}

func awaitFolder(feed *sidebarFeed, folderID, name string) {
	feed.t.Helper()
	feed.await(fmt.Sprintf("folder %s named %q", folderID, name), func(_ int, r feedRow) bool {
		return r.Kind == string(stream.KindFolder) && r.ID == folderID && !r.Gone && r.Data["name"] == name
	})
}

func folderIDs(
	rows []agentChatFolderDTO,
) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID)
	}
	return out
}

func assertDenseChatLevel(
	t *testing.T,
	h *harness,
	base string,
	container string,
	workspaceID string,
) {
	t.Helper()
	h.Quiesce()
	orders := map[string]int{}
	for _, f := range listChatFolders(t, h, base) {
		if f.ParentID == container {
			orders[f.ID] = f.Order
		}
	}
	var chats []agentChatDTO
	h.get(base+"/chats", &chats)
	for _, c := range chats {
		if c.Worktree != nil && c.Worktree.OwningChatID == c.ID {
			continue
		}
		if c.ParentID == container && (container != "" || c.WorkspaceID == workspaceID) {
			orders[c.ID] = c.Order
		}
	}
	seen := map[int]string{}
	for id, order := range orders {
		require.GreaterOrEqual(t, order, 0, "row %s", id)
		held, taken := seen[order]
		require.False(t, taken, "row %s: order %d is already held by %s", id, order, held)
		seen[order] = id
	}
	if container == "" {
		return
	}
	for id, order := range orders {
		require.Less(t, order, len(orders), "row %s: order %d is outside 0..%d", id, order, len(orders)-1)
	}
}
