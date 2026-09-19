//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deleting the owning chat of a worktree other chats still work in (the
// repo's default checkout, held by a thread in a root folder) purged the
// owner and left the worktree ownerless; the next read then ELECTED the
// untitled thread as owner and folded it into the header row.
func TestRegression_DeleteSharedWorktreeOwnerRemintsAnOwner(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProjectHomeHoldsDefault(t, h)
	base := repoBase(imported)
	defaultWS := defaultWorkspaceOf(t, h, imported)

	var folder struct {
		ID string `json:"id"`
	}
	h.post(base+"/chats/folders", map[string]any{"name": "root folder", "parentId": ""}, http.StatusCreated, &folder)
	status, threadID, errMsg := postChat(t, h, base,
		map[string]any{"provider": "promotestub", "workspaceId": defaultWS, "parentId": folder.ID})
	require.Equalf(t, http.StatusCreated, status, "thread in root folder: %d %s", status, errMsg)
	h.QuiesceReactors()
	h.Quiesce()

	ownerID := workspaceRow(t, h, imported, defaultWS).OwningChatID
	require.NotEmpty(t, ownerID)
	require.NotEqual(t, threadID, ownerID)

	frames := dialAgentWS(t, h, base+"/chats/ws")
	resp := h.raw(http.MethodDelete, base+"/chats/"+ownerID, nil, http.StatusAccepted)
	_ = resp.Body.Close()
	waitForChatFrame(t, frames, ownerID, "deleted")
	h.Quiesce()

	row := workspaceRow(t, h, imported, defaultWS)
	assert.NotEqual(t, "deleted", row.Status, "a worktree a surviving thread holds is not reaped")
	assert.NotEmpty(t, row.OwningChatID, "the worktree keeps an owner")
	assert.NotEqual(t, ownerID, row.OwningChatID)
	assert.NotEqual(t, threadID, row.OwningChatID, "the surviving thread is never elected owner")

	var chats []agentChatDTO
	h.get(base+"/chats", &chats)
	var thread *agentChatDTO
	for i := range chats {
		if chats[i].ID == threadID {
			thread = &chats[i]
		}
	}
	require.NotNil(t, thread, "the thread survives")
	if assert.NotNil(t, thread.Worktree) {
		assert.NotEqual(t, threadID, thread.Worktree.OwningChatID)
	}
}
