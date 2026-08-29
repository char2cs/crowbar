//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_SidebarForestMoveAndDelete exercises the unified sidebar
// forest end to end (2026-08-23 unified-sidebar-design + its backend
// addendum): a folder holding a nested pair of chats, moved to a new parent,
// refused a delete while a chat in the moved subtree is WORKING, and finally
// taken whole once the work is done — while the folder that held it survives
// untouched, matching the model's own opposite rule for the two deletes.
//
// Two disclosed departures from the task's literal recipe, both verified live
// (not assumed) against this exact codebase state, neither fixed here — this
// task is a gate/coverage/integration-suite pass, not new product work:
//
//  1. A literal BUBBLE chat (Chat.WorkspaceID == "", model spec §3.1) cannot
//     be created over the real HTTP surface today: POST .../chats with no
//     workspaceId 500s ("spawn runner: worktree dir: ... aggregate not
//     found"), because StartRunner/spawnPaths resolve a runner's cwd from the
//     chat's OWN WorkspaceID field directly (internal/app/usecases/chat/
//     internal/runner/spawn.go), never through the ancestor cwd walk
//     tree.CwdWorkspaceID describes (§3.2: "a bubble under develop runs in
//     develop's worktree"). Every unit test masks this with a fake
//     WorktreeDir that answers any id, including "" — only a real end-to-end
//     run against the real workspace repository surfaces it. And a chat
//     thread is refused across DIFFERENT workspaces regardless (checkParent-
//     Kind/ErrCrossWorkspace, pinned by TestRegression_ChatTreeRefusesCross-
//     WorkspaceParentage), so two DISTINCT worktree chats cannot be threaded
//     together either. So both chats below share ONE workspace — the second
//     THREADED under the first, exactly how every other thread fixture in
//     this suite (newThreadChat/newThreadChatUnder) already builds one —
//     rather than a true bubble, which still nests it inside the folder's
//     filed subtree the way the recipe intends.
//  2. The refusal is asserted on a CHAT delete, not a FOLDER delete: a folder
//     delete PROMOTES its children (it does not take them — the opposite
//     rule from a chat delete, see TestRegression_ChatFolderDeletePromotes-
//     ItsChildren) and its usecase (tree.go's Delete) never calls
//     guardNotWorking at all, unlike its own Move a few lines above in the
//     same file — so today a folder delete silently promotes a WORKING chat
//     filed under it instead of refusing. That is a real, reproducible gap in
//     invariant 5/9's enforcement, flagged for the final whole-branch review;
//     it is not something a test can honestly paper over by asserting a
//     refusal that does not happen. The chat-delete cascade (DeleteChat) DOES
//     enforce the guard correctly, which is what this test pins.
func TestRegression_SidebarForestMoveAndDelete(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)

	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	sprint := createChatFolder(t, h, base, "sprint", "")

	parentChat := createAgentChat(t, h, imported)
	placeChat(t, h, base, parentChat, map[string]any{"parentId": sprint.ID})
	h.Quiesce()

	childChat := createAgentChat(t, h, imported)
	placeChat(t, h, base, childChat, map[string]any{"parentId": parentChat})
	h.Quiesce()

	// The folder holds parentChat directly and childChat two levels down
	// (folder -> parentChat -> childChat) — the recipe's "two chats under it"
	// nested the way an actual thread files, rather than two siblings.
	var parentDetail, childDetail agentChatDetail
	h.get(base+"/chats/"+parentChat, &parentDetail)
	h.get(base+"/chats/"+childChat, &childDetail)
	require.Equal(t, sprint.ID, parentDetail.ParentID)
	require.Equal(t, parentChat, childDetail.ParentID)

	// Move the folder to a new destination: the chats filed under it travel
	// with it, because they hang off the FOLDER (and each other), never off a
	// copy of its old location.
	archive := createChatFolder(t, h, base, "archive", "")
	h.patch(base+"/chats/folders/"+sprint.ID, map[string]any{"parentId": archive.ID}, nil)
	h.Quiesce()

	rows := listChatFolders(t, h, base)
	sprintRow, ok := chatFolderByID(rows, sprint.ID)
	require.True(t, ok, "the moved folder must still exist")
	assert.Equal(t, archive.ID, sprintRow.ParentID, "the folder itself actually moved")

	h.get(base+"/chats/"+parentChat, &parentDetail)
	h.get(base+"/chats/"+childChat, &childDetail)
	assert.Equal(t, sprint.ID, parentDetail.ParentID, "the filed chat's own placement is untouched by the folder's move")
	assert.Equal(t, parentChat, childDetail.ParentID, "and so is the thread beneath it")

	// Open a turn on the innermost chat: the subtree rooted at parentChat is
	// now WORKING, and a chat delete refuses it unconditionally (backend
	// addendum §2, invariant 9) — there is no "delete anyway" the way a
	// locked-branch delete has.
	_ = h.raw(http.MethodPost, base+"/chats/hooks", map[string]string{
		"segment_id": childDetail.LiveRunnerID, "provider": "livestub", "event": "user_prompt",
		"payload_raw": `{"prompt":"still going"}`,
	}, http.StatusAccepted).Body.Close()
	h.Quiesce()

	msg := h.mutationError(http.MethodDelete, base+"/chats/"+parentChat, nil, http.StatusConflict)
	assert.Contains(t, msg, "working",
		"the refusal has to name why: a working row in the subtree refuses this verb unconditionally")

	// The refused delete changed nothing: both chats and the folder are
	// exactly where they were.
	var stillParent, stillChild agentChatDetail
	h.get(base+"/chats/"+parentChat, &stillParent)
	h.get(base+"/chats/"+childChat, &stillChild)
	assert.Equal(t, sprint.ID, stillParent.ParentID)
	assert.Equal(t, parentChat, stillChild.ParentID)

	// Close the turn: the subtree is idle again, and the SAME delete now
	// succeeds — taking parentChat AND the thread filed under it, the whole
	// set a move or delete always takes together (invariant 4).
	_ = h.raw(http.MethodPost, base+"/chats/hooks", map[string]string{
		"segment_id": childDetail.LiveRunnerID, "provider": "livestub", "event": "turn_stop",
		"payload_raw": `{"last_assistant_message":"done"}`,
	}, http.StatusAccepted).Body.Close()
	h.Quiesce()

	resp := h.raw(http.MethodDelete, base+"/chats/"+parentChat, nil, http.StatusAccepted)
	_ = resp.Body.Close()
	h.Quiesce()

	for _, chatID := range []string{parentChat, childChat} {
		getResp := h.raw(http.MethodGet, base+"/chats/"+chatID, nil, http.StatusNotFound)
		_ = getResp.Body.Close()
	}

	// The folder itself survives: a chat delete takes its OWN subtree, never
	// the folder that happened to hold it — the opposite rule a folder's own
	// delete follows for exactly the reverse reason (it holds no
	// conversation, so what it holds outlives it).
	rows = listChatFolders(t, h, base)
	_, stillThere := chatFolderByID(rows, sprint.ID)
	assert.True(t, stillThere, "the folder the deleted chat was filed in must survive the chat's own delete")
}
