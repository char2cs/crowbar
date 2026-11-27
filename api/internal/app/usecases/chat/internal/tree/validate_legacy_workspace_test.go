package tree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

func TestRegression_PlaceChat_UnderALegacyWorkspaceWithoutANodeRow(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	seedChat(chats, "c1", 1)

	placed, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1", tree.PlaceInput{ParentID: name(workspaceID)})
	require.NoError(t, err, "a legacy chatless workspace is a real container")
	assert.Equal(t, workspaceID, placed.ParentID)
	assert.Equal(t, dtree.KindWorkspace, nodeRowFor(t, nodes, workspaceID).Kind,
		"the anchor is minted by the first write under it")
}

func TestRegression_PlaceWorkspace_UnderALegacyWorkspaceWithoutANodeRow(t *testing.T) {
	_, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	gitStatus.SetRepo("legacy-ws", repoID)
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)
	gitStatus.SetForkParent("branch-1", "legacy-ws")
	nodes.Rows = append(nodes.Rows, dtree.Node{ID: "branch-1", Kind: dtree.KindWorkspace})

	placed, _, err := uc.PlaceWorkspace(context.Background(), "branch-1", tree.PlaceInput{ParentID: name("legacy-ws")})
	require.NoError(t, err)
	assert.Equal(t, "legacy-ws", placed.ParentID)
	assert.Equal(t, dtree.KindWorkspace, nodeRowFor(t, nodes, "legacy-ws").Kind)
}
