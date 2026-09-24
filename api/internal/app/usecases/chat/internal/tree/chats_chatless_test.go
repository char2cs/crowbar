package tree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The frontend sends a chatless workspace's OWN id as parentId (its row id):
// with a Node row the tree accepts it (workspaceAnchorType) and files the new
// chat under it; without one (a workspace created before Node minting, no
// backfill) the create mints that anchor row first and then proceeds.
func TestCreateChat_ThreadUnderAChatlessWorkspaceNode_IsPlacedUnderTheWorkspaceId(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	nodes.Rows = append(nodes.Rows, domain.Node{ID: workspaceID, Kind: domain.NodeKindWorkspace})
	chats.NextID = "c-new"

	chatID, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	assert.Equal(t, workspaceID, nodeRowFor(t, nodes, "c-new").ParentID)
}

func TestCreateChat_ForkUnderAChatlessWorkspaceNode_PlacesThenSpawns(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	nodes.Rows = append(nodes.Rows, domain.Node{ID: workspaceID, Kind: domain.NodeKindWorkspace})
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), "", "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeFork}, "")
	require.NoError(t, err)
	require.Len(t, chats.SpawnedOwnWorktree, 1)
	assert.Equal(t, workspaceID, chats.SpawnedOwnWorktree[0].ParentAtStart)
}

func TestRegression_CreateChat_UnderAChatlessWorkspaceWithNoNodeRow_MintsTheAnchor(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.NoError(t, err)
	anchor := nodeRowFor(t, nodes, workspaceID)
	assert.Equal(t, domain.NodeKindWorkspace, anchor.Kind)
	assert.Equal(t, workspaceID, nodeRowFor(t, nodes, "c-new").ParentID)

	chats.NextID = "c-fork"
	_, _, err = uc.CreateChat(context.Background(), "", "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeFork}, "")
	require.NoError(t, err)
	require.Len(t, chats.SpawnedOwnWorktree, 1)
	assert.Equal(t, workspaceID, chats.SpawnedOwnWorktree[0].ParentAtStart)
}
