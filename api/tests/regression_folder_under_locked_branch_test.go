//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegression_ChatMovedIntoFolderUnderLockedDefaultBranchStaysInFolder(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)

	folder := createChatFolder(t, h, repoBase(imported), "docs", imported.chatID)
	require.Equal(t, imported.chatID, folder.ParentID,
		"the folder must be created as a child of the locked chat, not root-normalised")

	listed := listChatFolders(t, h, repoBase(imported))
	got, ok := chatFolderByID(listed, folder.ID)
	require.True(t, ok, "the new folder must be listed back")
	require.Equal(t, imported.chatID, got.ParentID)

	c2 := createRootChatIn(t, h, imported, imported.workspaceID)

	placed := placeChat(t, h, repoBase(imported), c2, map[string]any{"parentId": folder.ID})
	require.Equal(t, folder.ID, placed.Chat.ParentID,
		"the chat must land inside the folder, not at the repo root or under the locked chat itself")
}

func TestRegression_ChatMovedIntoFolderUnderLockedNonDefaultBranchStaysInFolder(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	branchChatID := imported.chatID

	locked := true
	h.raw(http.MethodPost, repoBase(imported)+"/chats/"+branchChatID+"/lock",
		map[string]any{"locked": &locked}, http.StatusNoContent).Body.Close()
	h.Quiesce()

	folder := createChatFolder(t, h, repoBase(imported), "docs", branchChatID)
	require.Equal(t, branchChatID, folder.ParentID,
		"the folder must be created as a child of the locked branch's own chat, not root-normalised")

	c2 := createRootChatIn(t, h, imported, imported.workspaceID)

	placed := placeChat(t, h, repoBase(imported), c2, map[string]any{"parentId": folder.ID})
	require.Equal(t, folder.ID, placed.Chat.ParentID,
		"the chat must land inside the folder, not at the repo root or under the locked branch itself")
}
