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
//
// The tests below were unblocked by Task 5, which made WorkspaceID optional
// in the Create command. A folder is created with no workspace by design, and
// the Create command now supports this.

// agentChatFolderDTO mirrors the WIRE SHAPE a folder route answers with today:
// dto.AgentChatDTO (api/internal/api/v0/dto/agent.go), not the deleted
// AgentChatFolderDTO — a folder is a domain.Chat row now (Type == "folder"),
// rendered through the same converter a chat's own placement route already
// used, so its display name rides the "title" field.
type agentChatFolderDTO struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	ParentID    string `json:"parentId"`
	Title       string `json:"title"`
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

// chatIDs lists the chats a workspace currently has.
func chatIDs(
	t *testing.T,
	h *harness,
	base string,
) []string {
	t.Helper()
	h.Quiesce()
	var list []agentChatDTO
	h.get(base+"/chats", &list)
	// Conversations only: the branch rows every workspace now owns are sidebar
	// rows, not chats anybody opened. See conversationsOnly.
	list = conversationsOnly(list)
	out := make([]string, 0, len(list))
	for _, c := range list {
		out = append(out, c.ID)
	}
	return out
}

// chatOrderOf reads one chat's dense index off the LIST, which is the only
// place the order actually rides — the per-chat detail response carries the
// conversation history, not the row's placement.
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

// A thread exists to CONTINUE its parent — it reads that chat's turns — so a
// chat that is deleted takes every chat below it. Promoting them, the way a
// deleted folder promotes its children, would leave conversations whose entire
// premise has been erased and which no drag can restore.
//
// `bystander` is the second half, and the reason this test was quarantined for
// a spell: it shares ONE worktree with the subtree being deleted, exactly as
// ordinary sibling conversations do, and the cascade used to tear that worktree
// down on its way past — destroying the bystander and leaving nothing behind
// able to name the directory. The cascade now subtracts the doomed subtree from
// the worktree's holder census and reaps only what nothing else is holding
// (tree.reapWorktrees), so the assertion below is about both rules at once:
// everything threaded below the deleted chat goes, and NOTHING else does.
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

	// A folder INSIDE the doomed subtree, ordering the root chat's threads. It
	// holds no conversation, but with the chat it hung off gone there is nothing
	// left for it to order.
	inside := createChatFolder(t, h, base, "spikes", root)

	resp := h.raw(http.MethodDelete, base+"/chats/"+root, nil, http.StatusAccepted)
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

// The opposite rule, and the reason the two are worth pinning together: a folder
// holds no conversation, so the chats filed under it outlive it. Deleting them
// would destroy work the user only meant to unfile.
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

// A move into a row's own subtree would leave a set of rows unreachable from the
// panel root: they exist, nothing renders them, and nothing can drag them back
// out. For a CHAT it is worse — the parent is what the chat reads, so a chat
// inside its own subtree is a context walk that never reaches the root. Refused
// server-side, before any write.
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

	// The refused moves changed nothing.
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

// A CHAT parent is still refused across workspaces: a thread's parent is what
// it READS, so accepting one from a workspace the user is not in would let an
// agent inherit context. That refusal is decided from the MOVED chat's own
// actual workspace (checkChatContainer), so it survives Task 17's route
// rescope unchanged even though the repo-scoped mount no longer names a
// workspace in the URL at all.
//
// A FOLDER parent is not refused this way any more. A folder carries no
// workspace of its own now (2026-08-23 unified-sidebar-design §3.1) — it is a
// domain.Chat row of Type "folder" that lives in the repo forest, not inside
// one workspace's tree — so there is no workspace edge left on it to compare.
// The real boundary a folder-to-folder or chat-to-folder parentage should
// respect is the REPO the row lives in, and enforcing that is stage 3's walk
// (Chats.ListChats's own doc comment), not this task's storage retype. This
// test pins today's honest, permissive behaviour so a future tightening is a
// conscious assertion change here, not a silently discovered regression.
//
// Addressing a chat by id alone (Task 17, model spec §5.1) also retires the
// third case this test used to pin: a chat is no longer invisible merely
// because the URL used to reach it names a different repo/workspace. The
// repo-scoped mount has no :wsId segment to compare against, so
// requireChatInWorkspace's cross-workspace 404 only ever fires at the HOME
// mount now (RequireHomeWorkspace's injected :wsId) — pinned separately by
// TestAgentREST_Scope. Here, writing chat B's own placement through repo A's
// URL is legitimate access to a row addressed by id, not a probe into
// something the caller may not touch, and this test now pins THAT.
func TestRegression_ChatTreeRefusesCrossWorkspaceParentage(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	a := importWritableWorkspace(t, h)
	b := importWritableWorkspace(t, h)

	foreignFolder := createChatFolder(t, h, repoBase(b), "elsewhere", "")
	foreignChat := createAgentChat(t, h, b)

	// Filing a new folder under another repo's folder, or under another
	// workspace's chat, is accepted — the repo boundary is not enforced yet.
	created := createChatFolder(t, h, repoBase(a), "spikes", foreignFolder.ID)
	assert.Equal(t, foreignFolder.ID, created.ParentID)
	createChatFolder(t, h, repoBase(a), "spikes-2", foreignChat)

	// A CHAT thread is still refused across workspaces: this boundary is
	// unchanged by the retype, since a chat still carries a workspace and the
	// refusal is decided from the MOVED chat's own workspace, never the URL.
	own := createAgentChat(t, h, a)
	msg := h.mutationError(http.MethodPatch, repoBase(a)+"/chats/"+own+"/placement",
		map[string]string{"parentId": foreignChat}, http.StatusConflict)
	assert.Contains(t, msg, "workspace")

	// Renaming a folder addressed through a DIFFERENT repo's URL now succeeds
	// too: a folder is addressed by id alone, not id-within-workspace.
	h.patch(repoBase(a)+"/chats/folders/"+foreignFolder.ID, map[string]any{"name": "renamed"}, nil)

	// A chat addressed through a DIFFERENT repo's URL is now reachable and
	// mutable, not invisible: the repo-scoped mount has no workspace segment
	// left to be wrong about, so reparenting chat B through repo A's URL is
	// ordinary access to a row addressed by id, and it actually lands.
	placed := placeChat(t, h, repoBase(a), foreignChat, map[string]any{"parentId": foreignFolder.ID})
	assert.Equal(t, foreignFolder.ID, placed.Chat.ParentID,
		"the reparent, addressed through the other repo's mount, actually landed")

	var reread agentChatDetail
	h.get(repoBase(b)+"/chats/"+foreignChat, &reread)
	assert.Equal(t, foreignFolder.ID, reread.ParentID,
		"and is visible back through the chat's own repo mount too")
}

// Sibling order is a dense index chats and folders SHARE, rebuilt on every move.
// Dense is what makes the next drop index mean what it says — and the rows a
// renumber moved have to come back on the response, or every other client cache
// holds stale orders until it reconnects.
func TestRegression_ChatTreeOrderIsDenseAndReturnsWhatItShifted(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)

	chat := createAgentChat(t, h, ws)
	// The panel root already holds the BRANCH row every imported workspace owns,
	// so this level's indices no longer start at zero. Read where the chat
	// actually landed and assert every position RELATIVE to it: what this test
	// is about is that the level stays DENSE and that a renumber reports what it
	// moved, never the absolute slot a row happens to occupy.
	chatOrder := chatOrderOf(t, h, base, chat)

	first := createChatFolder(t, h, base, "a", "")
	require.Greater(t, first.Order, chatOrder,
		"a new folder lands after the chat already at that level")

	second := createChatFolder(t, h, base, "b", "")
	require.Equal(t, first.Order+1, second.Order, "and the next one lands after that")

	// Dragging the last folder to the top renumbers the whole level, and the
	// answer names every OTHER folder the renumber moved.
	var moved chatFolderMutation
	h.patch(base+"/chats/folders/"+second.ID, map[string]any{"order": 0}, &moved)
	assert.Equal(t, 0, moved.Folder.Order)
	shifted, ok := chatFolderByID(moved.Shifted, first.ID)
	require.True(t, ok, "the folder the drop pushed down must ride back with the answer")
	assert.Equal(t, first.Order+1, shifted.Order,
		"the drop pushed it down exactly one slot")
	assert.NotContains(t, folderIDs(moved.Shifted), second.ID, "the subject is not its own collateral")

	// The chat shares the level, so it was renumbered too — through its own
	// aggregate write, which is why it is absent from `shifted`.
	h.Quiesce()
	var reread agentChatDetail
	h.get(base+"/chats/"+chat, &reread)
	assert.Greater(t, reread.Order, chatOrder,
		"the drop landed above the chat, so the renumber pushed the chat down")

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
	require.Len(t, conversationsOnly(list), 1, "the home chat outlived the folder it was filed in")
}

// A folder mutation has no aggregate projection to ride, so the handler
// broadcasts it — on the SAME repo-scoped socket the chats use (Task 17), because
// one gesture writes both kinds and two feeds would have to be kept in order.
//
// The frame's workspaceId is empty here, not ws.workspaceID: a folder carries no
// workspace of its own (2026-08-23 unified-sidebar-design §3.1), and the
// repo-scoped mount has no :wsId path segment to source one from either — only
// the home mount's injected :wsId (RequireHomeWorkspace) ever stamps one on a
// folder frame, which is a scoping convenience for that one mount, not a fact
// about the folder.
func TestRegression_ChatFolderMutationsRideTheChatsStream(t *testing.T) {
	h := newHarness(t)
	ws := importWritableWorkspace(t, h)
	base := repoBase(ws)

	frames := dialAgentWS(t, h, base+"/chats/ws")

	folder := createChatFolder(t, h, base, "spikes", "")
	created := waitForFolderFrame(t, frames, folder.ID, "folder_created")
	assert.Empty(t, created["workspaceId"],
		"a folder carries no workspace of its own, and the repo-scoped mount has no :wsId to stamp one from")

	h.patch(base+"/chats/folders/"+folder.ID, map[string]any{"name": "experiments"}, nil)
	waitForFolderFrame(t, frames, folder.ID, "folder_updated")

	h.del(base+"/chats/folders/"+folder.ID, nil, http.StatusOK, nil)
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
	h.get(base+"/chats", &chats)
	for _, c := range chats {
		if c.ParentID == container {
			orders[c.ID] = c.Order
		}
	}
	// A row's index must be non-negative and must be held by nobody else. That
	// is the whole of what "dense" buys and the whole of what a bad renumber
	// breaks: the next drop index means what it says only if no two rows claim
	// the same slot.
	seen := map[int]string{}
	for id, order := range orders {
		require.GreaterOrEqual(t, order, 0, "row %s", id)
		held, taken := seen[order]
		require.False(t, taken, "row %s: order %d is already held by %s", id, order, held)
		seen[order] = id
	}
	// The 0..n-1 half is asserted only INSIDE a container, where the rows above
	// are the whole level. It cannot be asserted at the panel root, and that is
	// not a gap in this change: the root is ONE sibling space shared by every
	// repo in the project AND by the project home's own row, while the list
	// read above is repo-scoped and never serves the home's. A root level of
	// six rows therefore reads as five here, with one legitimate index above
	// the count. That was always true of a daemon that had rebooted — the boot
	// backfill mints exactly these rows — and is simply true immediately now
	// that a workspace and the chat owning it are created together.
	if container == "" {
		return
	}
	for id, order := range orders {
		require.Less(t, order, len(orders), "row %s: order %d is outside 0..%d", id, order, len(orders)-1)
	}
}
