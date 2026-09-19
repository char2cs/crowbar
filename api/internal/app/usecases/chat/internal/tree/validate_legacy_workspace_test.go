package tree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// A workspace created before Node rows existed (every pre-#179 production
// checkout and locked branch — no backfill by design) has no Node row and no
// owning chat, yet the sidebar files rows under it by its own workspace id.
// Creates mint its anchor first (ensureWorkspaceAnchor); a placement
// addressed under it went through resolveRow alone, which had no workspace
// tier and answered 404 for a container that plainly exists.
func TestRegression_PlaceChat_UnderALegacyWorkspaceWithoutANodeRow(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	seedChat(chats, "c1", 1)

	placed, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1", tree.PlaceInput{ParentID: name(workspaceID)})
	require.NoError(t, err, "a legacy chatless workspace is a real container")
	assert.Equal(t, workspaceID, placed.ParentID)
	assert.Equal(t, domain.NodeKindWorkspace, nodeRowFor(t, nodes, workspaceID).Kind,
		"the anchor is minted by the first write under it")
}

func TestRegression_PlaceWorkspace_UnderALegacyWorkspaceWithoutANodeRow(t *testing.T) {
	_, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	gitStatus.SetRepo("legacy-ws", repoID)
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)
	gitStatus.SetForkParent("branch-1", "legacy-ws")
	nodes.Rows = append(nodes.Rows, domain.Node{ID: "branch-1", Kind: domain.NodeKindWorkspace})

	placed, _, err := uc.PlaceWorkspace(context.Background(), "branch-1", tree.PlaceInput{ParentID: name("legacy-ws")})
	require.NoError(t, err)
	assert.Equal(t, "legacy-ws", placed.ParentID)
	assert.Equal(t, domain.NodeKindWorkspace, nodeRowFor(t, nodes, "legacy-ws").Kind)
}
