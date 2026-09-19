//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A workspace created before the chat-first mint existed has no owning chat,
// and every worktree verb, the files/git/terminal surfaces and the delete
// cascade are chat-keyed — a client could not even address it. There is no
// boot backfill by design, so the first READ mints the owner instead.
func TestRegression_ChatlessWorkspaceIsServedWithAnOwner(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	lockedID := chatlessLockedBranch(t, h, imported, "release/chatless")

	first := workspaceRow(t, h, imported, lockedID)
	require.NotEmpty(t, first.OwningChatID, "the first read mints an owner")
	h.Quiesce()

	second := workspaceRow(t, h, imported, lockedID)
	assert.Equal(t, first.OwningChatID, second.OwningChatID, "the mint is idempotent")

	// The owner is a real, addressable row: the chat list marks it as owning
	// this worktree, and it is filed under the parent workspace's row exactly
	// as a freshly created locked branch's owner would be.
	var rows []agentChatDTO
	h.get(repoBase(imported)+"/chats", &rows)
	var owner *agentChatDTO
	for i := range rows {
		if rows[i].ID == first.OwningChatID {
			owner = &rows[i]
		}
	}
	require.NotNil(t, owner, "the minted owner is listed")
	require.NotNil(t, owner.Worktree)
	assert.Equal(t, owner.ID, owner.Worktree.OwningChatID)
	assert.Equal(t, lockedID, owner.WorkspaceID)
	assert.Equal(t, imported.workspaceID, owner.ParentID)

	// A thread started inside the workspace stays its own row: the owner is
	// recorded, so ResolveOwningChat never promotes the thread instead.
	var thread struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]any{"provider": "promotestub", "parentId": first.OwningChatID, "workspaceId": lockedID},
		http.StatusCreated, &thread)
	h.QuiesceReactors()
	after := workspaceRow(t, h, imported, lockedID)
	assert.Equal(t, first.OwningChatID, after.OwningChatID)
	detail := getAgentChat(t, h, repoBase(imported), thread.ID)
	if assert.NotNil(t, detail.Worktree) {
		assert.Equal(t, first.OwningChatID, detail.Worktree.OwningChatID, "the thread is not the owner")
	}
}

// A legacy default checkout with neither an owner nor a Node row is minted
// one at the root, and the owner is then addressable by the chat-keyed verbs.
func TestRegression_LegacyDefaultCheckoutIsServedWithAnOwner(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	ctx := context.Background()
	require.NoError(t, h.app.Usecases.AgentChat.PurgeChat(ctx, imported.chatID))
	require.NoError(t, h.app.Repositories.Node.Forget(ctx, imported.workspaceID))
	h.Quiesce()

	row := workspaceRow(t, h, imported, imported.workspaceID)
	require.NotEmpty(t, row.OwningChatID)
	h.Quiesce()

	detail := getAgentChat(t, h, repoBase(imported), row.OwningChatID)
	assert.Equal(t, imported.workspaceID, detail.WorkspaceID)
	assert.Equal(t, "", detail.ParentID, "a default checkout's owner sits at the repo root")
}

// The project-home owner rides no repo, and the home chat list used to be
// served with no worktree enrichment at all — so nothing on the wire said
// which home chat owned the home workspace, and the client drew the owner as
// an "Untitled chat" ghost row.
func TestRegression_HomeOwnerCarriesItsWorktreeOnTheWire(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	var home struct {
		ID           string `json:"id"`
		OwningChatID string `json:"owningChatId"`
	}
	h.get("/v0/projects/"+imported.projectID+"/home", &home)
	require.NotEmpty(t, home.OwningChatID)

	var rows []agentChatDTO
	h.get("/v0/projects/"+imported.projectID+"/home/chats", &rows)
	var owner *agentChatDTO
	for i := range rows {
		if rows[i].ID == home.OwningChatID {
			owner = &rows[i]
		}
	}
	require.NotNil(t, owner)
	require.NotNil(t, owner.Worktree, "the home owner's row carries its worktree")
	assert.Equal(t, owner.ID, owner.Worktree.OwningChatID)
	assert.Equal(t, home.ID, owner.WorkspaceID)
}
