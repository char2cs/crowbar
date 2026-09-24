package tree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// An UNLOCKED chatless workspace's own Node row is never merged into the
// placement snapshot (mergeHomeNode keeps only RendersAsBranch anchors) and so
// is never queued as a container: the threads already filed under it are
// invisible to the densify, and every new thread lands at order 0 beside them.
func TestRegression_CreateChat_SecondThreadUnderAnUnlockedChatlessWorkspace_GetsTheNextSlot(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	nodes.Rows = append(nodes.Rows, domain.Node{ID: workspaceID, Kind: domain.NodeKindWorkspace})
	ctx := context.Background()

	chats.NextID = "c-first"
	_, _, err := uc.CreateChat(ctx, workspaceID, "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.NoError(t, err)
	chats.NextID = "c-second"
	_, _, err = uc.CreateChat(ctx, workspaceID, "claude", workspaceID, tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.NoError(t, err)

	first := nodeRowFor(t, nodes, "c-first")
	second := nodeRowFor(t, nodes, "c-second")
	assert.Equal(t, workspaceID, first.ParentID)
	assert.Equal(t, workspaceID, second.ParentID)
	assert.Equal(t, 0, first.Order)
	assert.Equal(t, 1, second.Order, "the second thread is appended after the first, not tied with it")
}
