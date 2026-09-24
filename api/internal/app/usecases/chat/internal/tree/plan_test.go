package tree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// A locked branch's own row is a Node write with no projection frame of its
// own, so the only way a client learns its order moved is the `shifted` list
// of the drop that moved it. persist used to report folder rows alone, and
// a chat dropped before a locked branch left the branch tied at the chat's
// old order on every client — drawn first by the kind tie-break, the drop
// visibly undone.
func TestRegression_PlaceChat_ReportsAShiftedWorkspaceAnchorSibling(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "branch-1", Type: domain.ChatTypeBranch, OwnsWorkspace: true, WorkspaceID: "ws-branch-1"},
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: workspaceID},
	)
	nodes.Rows = append(nodes.Rows,
		domain.Node{ID: "ws-branch-1", Kind: domain.NodeKindWorkspace, Order: 0},
		domain.Node{ID: "c1", Kind: domain.NodeKindChat, Order: 1},
	)
	gitStatus.SetRepo("ws-branch-1", repoID)
	gitStatus.SetBranch("ws-branch-1", true)
	gitStatus.SetRepo(workspaceID, repoID)
	gitStatus.SetDefault(repoID, workspaceID)

	placed, shifted, err := uc.PlaceChat(context.Background(), workspaceID, "c1", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, 0, placed.Order)
	assert.Equal(t, 1, nodeRowFor(t, nodes, "ws-branch-1").Order, "the branch's anchor is renumbered past the chat")
	require.Len(t, shifted, 1, "the shifted anchor must be announced")
	assert.Equal(t, "ws-branch-1", shifted[0].ID)
	assert.Equal(t, 1, shifted[0].Order)
}

// The same drop the other way round: a locked branch placed before a chat
// reports the chat it shifted through the chat's own aggregate frame, and
// PlaceWorkspace keeps reporting the folder rows it shifted.
func TestRegression_PlaceWorkspace_ReportsAShiftedWorkspaceAnchorSibling(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "branch-1", Type: domain.ChatTypeBranch, OwnsWorkspace: true, WorkspaceID: "ws-branch-1"},
		domain.Chat{ID: "branch-2", Type: domain.ChatTypeBranch, OwnsWorkspace: true, WorkspaceID: "ws-branch-2"},
	)
	nodes.Rows = append(nodes.Rows,
		domain.Node{ID: "ws-branch-1", Kind: domain.NodeKindWorkspace, Order: 0},
		domain.Node{ID: "ws-branch-2", Kind: domain.NodeKindWorkspace, Order: 1},
	)
	for _, ws := range []string{"ws-branch-1", "ws-branch-2"} {
		gitStatus.SetRepo(ws, repoID)
		gitStatus.SetBranch(ws, true)
	}

	_, shifted, err := uc.PlaceWorkspace(context.Background(), "ws-branch-2", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, 1, nodeRowFor(t, nodes, "ws-branch-1").Order)
	require.Len(t, shifted, 1)
	assert.Equal(t, "ws-branch-1", shifted[0].ID)
	assert.Equal(t, 1, shifted[0].Order)
}
