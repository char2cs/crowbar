package tree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

func TestCreateChat_ThreadUnderAChatlessWorkspaceNode_IsPlacedUnderTheWorkspaceId(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	nodes.Rows = append(nodes.Rows, dtree.Node{ID: workspaceID, Kind: dtree.KindWorkspace})
	chats.NextID = "c-new"

	chatID, _, err := createChat(context.Background(), uc, workspaceID, "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	assert.Equal(t, workspaceID, nodeRowFor(t, nodes, "c-new").ParentID)
}

func TestCreateChat_ForkUnderAChatlessWorkspaceNode_PlacesThenSpawns(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	nodes.Rows = append(nodes.Rows, dtree.Node{ID: workspaceID, Kind: dtree.KindWorkspace})
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, "", "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeFork}, "")
	require.NoError(t, err)
	require.Len(t, chats.SpawnedOwnWorktree, 1)
	assert.Equal(t, workspaceID, chats.SpawnedOwnWorktree[0].ParentAtStart)
}

func TestRegression_CreateChat_UnderAChatlessWorkspaceWithNoNodeRow_MintsTheAnchor(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.NoError(t, err)
	anchor := nodeRowFor(t, nodes, workspaceID)
	assert.Equal(t, dtree.KindWorkspace, anchor.Kind)
	assert.Equal(t, workspaceID, nodeRowFor(t, nodes, "c-new").ParentID)

	chats.NextID = "c-fork"
	_, _, err = createChat(context.Background(), uc, "", "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeFork}, "")
	require.NoError(t, err)
	require.Len(t, chats.SpawnedOwnWorktree, 1)
	assert.Equal(t, workspaceID, chats.SpawnedOwnWorktree[0].ParentAtStart)
}

func createChat(
	ctx context.Context,
	uc tree.Usecase,
	workspaceID, providerID, parentID string,
	worktree tree.WorktreeSpec,
	surface string,
) (string, string, error) {
	created, err := uc.CreateChat(ctx, tree.CreateChatInput{
		WorkspaceID: workspaceID, ProviderID: providerID, ParentID: parentID, Worktree: worktree, Surface: surface,
	})
	return created.ChatID, created.RunnerID, err
}
