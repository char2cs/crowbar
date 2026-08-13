//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file pins the Chats panel's tree end to end: the folders a user files
// chats into, the dense order chats and folders SHARE, the two guards that keep
// the tree renderable (no cycles, no cross-workspace edges), and the two deletes
// that are deliberately opposite — a folder promotes what it held, a chat takes
// its whole subtree. Each test drives the real HTTP surface, so a rule that only
// ever existed in the frontend would fail here.

// agentChatFolderDTO mirrors the AgentChatFolderDTO wire shape.
type agentChatFolderDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	ParentID    string `json:"parentId"`
	Name        string `json:"name"`
	Order       int    `json:"order"`
}

// chatFolderMutation mirrors the { folder, shifted } body every chat-folder
// mutation answers with.
type chatFolderMutation struct {
	Folder  agentChatFolderDTO   `json:"folder"`
	Shifted []agentChatFolderDTO `json:"shifted"`
}

// placedChatDTO mirrors the { chat, shifted } body the placement route answers
// with. The chat carries its lineage, because in this panel the parent IS the
// lineage.
type placedChatDTO struct {
	Chat struct {
		ID       string `json:"id"`
		ParentID string `json:"parentId"`
		Order    int    `json:"order"`
	} `json:"chat"`
	Shifted []agentChatFolderDTO `json:"shifted"`
}

// createChatFolder posts a folder under base and returns the created row.
func createChatFolder(
	t *testing.T,
	h *harness,
	base string,
	name string,
	parentID string,
) agentChatFolderDTO {
	t.Helper()
	var out chatFolderMutation
	h.post(base+"/agent/folders",
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
	h.get(base+"/agent/folders", &folders)
	return folders
}

// placeChat moves a chat and returns the answer.
func placeChat(
	t *testing.T,
	h *harness,
	base string,
	chatID string,
	body map[string]any,
) placedChatDTO {
	t.Helper()
	var out placedChatDTO
	h.patch(base+"/agent/chats/"+chatID+"/placement", body, &out)
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

// chatIDs lists the chats a workspace currently has.
func chatIDs(
	t *testing.T,
	h *harness,
	base string,
) []string {
	t.Helper()
	h.Quiesce()
	var list []agentChatDTO
	h.get(base+"/agent/chats", &list)
	out := make([]string, 0, len(list))
	for _, c := range list {
		out = append(out, c.ID)
	}
	return out
}

// A thread exists to CONTINUE its parent — it reads that chat's turns — so a
// chat that is deleted takes every chat below it. Promoting them, the way a
// deleted folder promotes its children, would leave conversations whose entire
// premise has been erased and which no drag can restore.
func TestRegression_ChatDeleteCascadesToItsThreads(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := wsBase(ws)

	root := createAgentChat(t, h, ws)
	child := createAgentChat(t, h, ws)
	grandchild := createAgentChat(t, h, ws)
	bystander := createAgentChat(t, h, ws)

	placeChat(t, h, base, child, map[string]any{"parentId": root})
	h.Quiesce()
	placeChat(t, h, base, grandchild, map[string]any{"parentId": child})
	h.Quiesce()

	// A folder INSIDE the doomed subtree, ordering the root chat's threads. It
	// holds no conversation, but with the chat it hung off gone there is nothing
	// left for it to order.
	inside := createChatFolder(t, h, base, "spikes", root)

	resp := h.raw(http.MethodDelete, base+"/agent/chats/"+root, nil, http.StatusAccepted)
	_ = resp.Body.Close()
	h.Quiesce()

	remaining := chatIDs(t, h, base)
	assert.Equal(t, []string{bystander}, remaining,
		"a deleted chat takes every chat threaded below it, and nothing else")

	_, found := chatFolderByID(listChatFolders(t, h, base), inside.ID)
	assert.False(t, found, "a folder caught inside the deleted subtree goes with it")

	getResp := h.raw(http.MethodGet, base+"/agent/chats/"+grandchild, nil, http.StatusNotFound)
	_ = getResp.Body.Close()
}

// The opposite rule, and the reason the two are worth pinning together: a folder
// holds no conversation, so the chats filed under it outlive it. Deleting them
// would destroy work the user only meant to unfile.
func TestRegression_ChatFolderDeletePromotesItsChildren(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := wsBase(ws)

	outer := createChatFolder(t, h, base, "outer", "")
	inner := createChatFolder(t, h, base, "inner", outer.ID)
	chat := createAgentChat(t, h, ws)
	placeChat(t, h, base, chat, map[string]any{"parentId": outer.ID})
	h.Quiesce()

	var deleted struct {
		Shifted []agentChatFolderDTO `json:"shifted"`
	}
	h.del(base+"/agent/folders/"+outer.ID, nil, http.StatusOK, &deleted)
	h.Quiesce()

	rows := listChatFolders(t, h, base)
	_, gone := chatFolderByID(rows, outer.ID)
	assert.False(t, gone, "the folder itself is removed")
	promoted, ok := chatFolderByID(rows, inner.ID)
	require.True(t, ok, "the child folder survives its parent")
	assert.Equal(t, "", promoted.ParentID, "and rises to the folder's own parent")

	var list []agentChatDTO
	h.get(base+"/agent/chats", &list)
	require.Len(t, list, 1)
	assert.Equal(t, chat, list[0].ID, "the chat outlives the folder that held it")
	assert.Equal(t, "", list[0].ParentID, "and is promoted, never deleted")
}

// A move into a row's own subtree would leave a set of rows unreachable from the
// panel root: they exist, nothing renders them, and nothing can drag them back
// out. For a CHAT it is worse — the parent is what the chat reads, so a chat
// inside its own subtree is a context walk that never reaches the root. Refused
// server-side, before any write.
func TestRegression_ChatTreeMoveRefusedWhenItWouldCycle(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := wsBase(ws)

	outer := createChatFolder(t, h, base, "outer", "")
	inner := createChatFolder(t, h, base, "inner", outer.ID)

	msg := h.mutationError(http.MethodPatch, base+"/agent/folders/"+outer.ID,
		map[string]string{"parentId": inner.ID}, http.StatusConflict)
	assert.Contains(t, msg, "inside itself",
		"the refusal has to say what is wrong, not just refuse")

	h.mutationError(http.MethodPatch, base+"/agent/folders/"+outer.ID,
		map[string]string{"parentId": outer.ID}, http.StatusConflict)

	parent := createAgentChat(t, h, ws)
	thread := createAgentChat(t, h, ws)
	placeChat(t, h, base, thread, map[string]any{"parentId": parent})
	h.Quiesce()

	h.mutationError(http.MethodPatch, base+"/agent/chats/"+parent+"/placement",
		map[string]string{"parentId": thread}, http.StatusConflict)

	// The refused moves changed nothing.
	rows := listChatFolders(t, h, base)
	outerRow, ok := chatFolderByID(rows, outer.ID)
	require.True(t, ok)
	assert.Equal(t, "", outerRow.ParentID)
	innerRow, ok := chatFolderByID(rows, inner.ID)
	require.True(t, ok)
	assert.Equal(t, outer.ID, innerRow.ParentID)

	var reread agentChatDetail
	h.get(base+"/agent/chats/"+parent, &reread)
	assert.Equal(t, "", reread.ParentID, "a refused placement must never rewrite lineage")
}

// The panel renders ONE workspace's tree, so a cross-workspace edge is a row
// nothing will ever draw — and for a chat it would additionally mean reading
// turns out of a workspace the user is not in.
func TestRegression_ChatTreeRefusesCrossWorkspaceParentage(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	a := importWritableWorkspace(t, h)
	b := importWritableWorkspace(t, h)

	foreignFolder := createChatFolder(t, h, wsBase(b), "elsewhere", "")
	foreignChat := createAgentChat(t, h, b)

	msg := h.mutationError(http.MethodPost, wsBase(a)+"/agent/folders",
		map[string]string{"name": "spikes", "parentId": foreignFolder.ID}, http.StatusConflict)
	assert.Contains(t, msg, "workspace")

	h.mutationError(http.MethodPost, wsBase(a)+"/agent/folders",
		map[string]string{"name": "spikes", "parentId": foreignChat}, http.StatusConflict)

	own := createAgentChat(t, h, a)
	h.mutationError(http.MethodPatch, wsBase(a)+"/agent/chats/"+own+"/placement",
		map[string]string{"parentId": foreignChat}, http.StatusConflict)

	// A row addressed through the WRONG workspace is not merely refused, it is
	// invisible: answering anything else would confirm a row the caller may not
	// touch exists.
	h.mutationError(http.MethodPatch, wsBase(a)+"/agent/folders/"+foreignFolder.ID,
		map[string]string{"name": "stolen"}, http.StatusNotFound)
	h.mutationError(http.MethodPatch, wsBase(a)+"/agent/chats/"+foreignChat+"/placement",
		map[string]any{"order": 0}, http.StatusNotFound)

	assert.Empty(t, listChatFolders(t, h, wsBase(a)), "no folder was created in the other workspace's name")
}

// Sibling order is a dense index chats and folders SHARE, rebuilt on every move.
// Dense is what makes the next drop index mean what it says — and the rows a
// renumber moved have to come back on the response, or every other client cache
// holds stale orders until it reconnects.
func TestRegression_ChatTreeOrderIsDenseAndReturnsWhatItShifted(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := wsBase(ws)

	chat := createAgentChat(t, h, ws)
	first := createChatFolder(t, h, base, "a", "")
	require.Equal(t, 1, first.Order, "a new folder lands after the chat already at that level")

	second := createChatFolder(t, h, base, "b", "")
	require.Equal(t, 2, second.Order)

	// Dragging the last folder to the top renumbers the whole level, and the
	// answer names every OTHER folder the renumber moved.
	var moved chatFolderMutation
	h.patch(base+"/agent/folders/"+second.ID, map[string]any{"order": 0}, &moved)
	assert.Equal(t, 0, moved.Folder.Order)
	shifted, ok := chatFolderByID(moved.Shifted, first.ID)
	require.True(t, ok, "the folder the drop pushed down must ride back with the answer")
	assert.Equal(t, 2, shifted.Order)
	assert.NotContains(t, folderIDs(moved.Shifted), second.ID, "the subject is not its own collateral")

	// The chat shares the level, so it was renumbered too — through its own
	// aggregate write, which is why it is absent from `shifted`.
	h.Quiesce()
	var reread agentChatDetail
	h.get(base+"/agent/chats/"+chat, &reread)
	assert.Equal(t, 1, reread.Order)

	assertDenseChatLevel(t, h, base, "")

	// A chat drop is the other half of the same gesture, and it renumbers the same
	// shared level — so it reports the folders it moved.
	placed := placeChat(t, h, base, chat, map[string]any{"order": 0})
	assert.Equal(t, 0, placed.Chat.Order)
	assert.NotEmpty(t, placed.Shifted, "a chat drop reports the folders it renumbered")
	assertDenseChatLevel(t, h, base, "")
}

// The project home accumulates more chats than any worktree workspace, so it is
// exactly the surface that most needs folders — and exactly the one a single
// mount would have left without them.
func TestRegression_ChatFoldersWorkOnHomeWorkspace(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := "/v0/projects/" + imported.projectID + "/home"

	var chat struct {
		ID string `json:"id"`
	}
	h.post(base+"/agent/chats", map[string]string{"provider": "stub"}, http.StatusCreated, &chat)
	require.NotEmpty(t, chat.ID)
	h.Quiesce()

	folder := createChatFolder(t, h, base, "spikes", "")
	rows := listChatFolders(t, h, base)
	require.Len(t, rows, 1)
	assert.Equal(t, "spikes", rows[0].Name)

	placed := placeChat(t, h, base, chat.ID, map[string]any{"parentId": folder.ID})
	assert.Equal(t, folder.ID, placed.Chat.ParentID, "a home chat files into a home folder")

	h.patch(base+"/agent/folders/"+folder.ID, map[string]any{"name": "experiments"}, nil)
	renamed := listChatFolders(t, h, base)
	require.Len(t, renamed, 1)
	assert.Equal(t, "experiments", renamed[0].Name)

	h.del(base+"/agent/folders/"+folder.ID, nil, http.StatusOK, nil)
	assert.Empty(t, listChatFolders(t, h, base))

	h.Quiesce()
	var list []agentChatDTO
	h.get(base+"/agent/chats", &list)
	require.Len(t, list, 1, "the home chat outlived the folder it was filed in")
}

// A folder mutation has no aggregate projection to ride, so the handler
// broadcasts it — on the SAME workspace-scoped socket the chats use, because one
// gesture writes both kinds and two feeds would have to be kept in order.
func TestRegression_ChatFolderMutationsRideTheChatsStream(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := wsBase(ws)

	frames := dialAgentWS(t, h, base+"/agent/ws/chats")

	folder := createChatFolder(t, h, base, "spikes", "")
	created := waitForFolderFrame(t, frames, folder.ID, "folder_created")
	assert.Equal(t, ws.workspaceID, created["workspaceId"])

	h.patch(base+"/agent/folders/"+folder.ID, map[string]any{"name": "experiments"}, nil)
	waitForFolderFrame(t, frames, folder.ID, "folder_updated")

	h.del(base+"/agent/folders/"+folder.ID, nil, http.StatusOK, nil)
	waitForFolderFrame(t, frames, folder.ID, "folder_deleted")
}

// waitForFolderFrame drains frames until one matching (folderID, kind) arrives,
// tolerating any other frame in between — a real signal wait, never a
// sleep/poll, backstopped only by the test context's Done().
func waitForFolderFrame(
	t *testing.T,
	frames <-chan map[string]any,
	folderID string,
	kind string,
) map[string]any {
	t.Helper()
	for {
		select {
		case f := <-frames:
			if f["folderId"] == folderID && f["kind"] == kind {
				return f
			}
		case <-t.Context().Done():
			t.Fatalf("timed out waiting for folder %s's %q frame", folderID, kind)
			return nil
		}
	}
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

// assertDenseChatLevel pins the whole point of the index: every row at a level —
// chat or folder, because they share it — holds a distinct 0..n-1 slot, so the
// next drop index means what it says.
func assertDenseChatLevel(
	t *testing.T,
	h *harness,
	base string,
	container string,
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
	h.get(base+"/agent/chats", &chats)
	for _, c := range chats {
		if c.ParentID == container {
			orders[c.ID] = c.Order
		}
	}
	seen := make([]bool, len(orders))
	for id, order := range orders {
		require.GreaterOrEqual(t, order, 0, "row %s", id)
		require.Less(t, order, len(orders), "row %s: order %d is outside 0..%d", id, order, len(orders)-1)
		require.False(t, seen[order], "row %s: order %d is held twice", id, order)
		seen[order] = true
	}
}
