package tree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The sidebar ids a CHATLESS workspace's row (a repo's default checkout, an
// untouched locked branch — PR #181's whole subject) by the WORKSPACE id, and
// its Thread "+" posts that id as parentId (space-content-actions.ts
// handleCreate: `createChat(wsId, provider, parentId)`).
func TestRegression_ThreadUnderAChatlessWorkspaceWithItsNodeRow(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	_, err := nodes.Create(context.Background(), workspaceID, domain.NodeKindWorkspace, "", 0)
	require.NoError(t, err)
	chats.NextID = "c-new"

	_, _, err = uc.CreateChat(context.Background(), workspaceID, "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeNone})
	assert.NoError(t, err)
}

// A workspace created before the Node forest existed (every pre-existing
// production workspace — no backfill by design) has NO Node row and no
// owning chat: the exact row the user's nightly shows.
func TestRegression_ThreadUnderALegacyChatlessWorkspaceWithoutANodeRow(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeNone})
	assert.NoError(t, err, "thread on a legacy chatless workspace row must not be refused")
	assert.NotEmpty(t, chats.Minted)
}

func TestRegression_ForkUnderALegacyChatlessWorkspaceWithoutANodeRow(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), "", "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeFork, Branch: "test/test"})
	assert.NoError(t, err, "fork off a legacy chatless workspace row must not be refused")
}

func TestRegression_ForkUnderAChatlessWorkspaceWithItsNodeRow(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	_, err := nodes.Create(context.Background(), workspaceID, domain.NodeKindWorkspace, "", 0)
	require.NoError(t, err)
	chats.NextID = "c-new"

	_, _, err = uc.CreateChat(context.Background(), "", "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeFork, Branch: "test/test"})
	assert.NoError(t, err)
}

// A fork cut through the workspace usecase (import, locked-branch provision)
// mints its owning row under the PARENT workspace's anchor Node. A legacy
// parent with no Node row dropped that row at the panel root instead.
func TestRegression_MintOwningChat_UnderALegacyWorkspaceWithoutANodeRow(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	chats.NextID = "owner-new"

	chatID, err := uc.MintOwningChat(context.Background(), workspaceID)
	require.NoError(t, err)
	assert.Equal(t, domain.NodeKindWorkspace, nodeRowFor(t, nodes, workspaceID).Kind, "the parent's anchor is minted on first touch")
	assert.Equal(t, workspaceID, chatRow(t, chats, chatID).ParentID)
}
