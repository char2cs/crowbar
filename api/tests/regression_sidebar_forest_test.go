//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	var parentDetail, childDetail agentChatDetail
	h.get(base+"/chats/"+parentChat, &parentDetail)
	h.get(base+"/chats/"+childChat, &childDetail)
	require.Equal(t, sprint.ID, parentDetail.ParentID)
	require.Equal(t, parentChat, childDetail.ParentID)

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

	childRunner := startChat(t, h, base, childChat)
	postHook(t, h, base, "livestub", childRunner, "user_prompt", `{"prompt":"still going"}`)
	h.Quiesce()

	folderMsg := h.mutationError(http.MethodDelete, base+"/chats/folders/"+sprint.ID, nil, http.StatusConflict)
	assert.Contains(t, folderMsg, "working",
		"a folder delete refuses a working row in its subtree exactly like a chat delete does")

	msg := h.mutationError(http.MethodDelete, base+"/chats/"+parentChat, nil, http.StatusConflict)
	assert.Contains(t, msg, "working",
		"the refusal has to name why: a working row in the subtree refuses this verb unconditionally")

	var stillParent, stillChild agentChatDetail
	h.get(base+"/chats/"+parentChat, &stillParent)
	h.get(base+"/chats/"+childChat, &stillChild)
	assert.Equal(t, sprint.ID, stillParent.ParentID)
	assert.Equal(t, parentChat, stillChild.ParentID)
	rows = listChatFolders(t, h, base)
	_, folderStillThere := chatFolderByID(rows, sprint.ID)
	assert.True(t, folderStillThere, "the refused folder delete promoted nothing")

	postHook(t, h, base, "livestub", childRunner, "turn_stop", `{"last_assistant_message":"done"}`)
	h.Quiesce()

	resp := h.raw(http.MethodDelete, base+"/chats/"+parentChat, nil, http.StatusOK)
	_ = resp.Body.Close()
	h.Quiesce()

	for _, chatID := range []string{parentChat, childChat} {
		getResp := h.raw(http.MethodGet, base+"/chats/"+chatID, nil, http.StatusNotFound)
		_ = getResp.Body.Close()
	}

	rows = listChatFolders(t, h, base)
	_, stillThere := chatFolderByID(rows, sprint.ID)
	assert.True(t, stillThere, "the folder the deleted chat was filed in must survive the chat's own delete")
}
