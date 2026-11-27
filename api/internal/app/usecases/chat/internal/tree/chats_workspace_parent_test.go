package tree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

func TestRegression_ThreadUnderAChatlessWorkspaceWithItsNodeRow(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	_, err := nodes.Create(context.Background(), workspaceID, dtree.KindWorkspace, "", 0)
	require.NoError(t, err)
	chats.NextID = "c-new"

	_, _, err = createChat(context.Background(), uc, workspaceID, "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	assert.NoError(t, err)
}

func TestRegression_ThreadUnderALegacyChatlessWorkspaceWithoutANodeRow(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	assert.NoError(t, err, "thread on a legacy chatless workspace row must not be refused")
	assert.NotEmpty(t, chats.Minted)
}

func TestRegression_ForkUnderALegacyChatlessWorkspaceWithoutANodeRow(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, "", "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeFork, Branch: "test/test"}, "")
	assert.NoError(t, err, "fork off a legacy chatless workspace row must not be refused")
}

func TestRegression_ForkUnderAChatlessWorkspaceWithItsNodeRow(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	_, err := nodes.Create(context.Background(), workspaceID, dtree.KindWorkspace, "", 0)
	require.NoError(t, err)
	chats.NextID = "c-new"

	_, _, err = createChat(context.Background(), uc, "", "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeFork, Branch: "test/test"}, "")
	assert.NoError(t, err)
}

func TestRegression_MintOwningChat_UnderALegacyWorkspaceWithoutANodeRow(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	chats.NextID = "owner-new"

	chatID, err := uc.MintOwningChat(context.Background(), "", workspaceID)
	require.NoError(t, err)
	assert.Equal(t, dtree.KindWorkspace, nodeRowFor(t, nodes, workspaceID).Kind, "the parent's anchor is minted on first touch")
	assert.Equal(t, workspaceID, chatRow(t, chats, chatID).ParentID)
}
