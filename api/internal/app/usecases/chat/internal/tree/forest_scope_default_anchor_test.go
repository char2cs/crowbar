package tree_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

func TestRegression_CreateChat_RepoRootLevelNeverCountsTheDefaultCheckoutAnchor(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	gitStatus.SetRepo(workspaceID, repoID)
	gitStatus.SetDefault(repoID, workspaceID)
	gitStatus.SetBranch(workspaceID, true)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "owner", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, OwnsWorkspace: true, CreatedAt: time.Unix(0, 0)},
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, ParentID: "owner", CreatedAt: time.Unix(1, 0)},
	)
	nodes.Rows = []dtree.Node{
		{ID: workspaceID, Kind: dtree.KindWorkspace, ParentID: "", Order: 0},
		{ID: "c1", Kind: dtree.KindChat, ParentID: "owner", Order: 0},
	}
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "owner", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.NoError(t, err)
	n := nodeRowFor(t, nodes, "c-new")
	assert.Equal(t, "owner", n.ParentID)
	assert.Equal(t, 1, n.Order, "one real sibling (c1) sits at the repo root, so the thread appends at 1")
	assert.False(t, writesTo(nodes, workspaceID), "the default checkout's own anchor is the header, never a root sibling to renumber")
	assert.False(t, writesTo(nodes, "c1"), "an already dense level is not renumbered by a create")
}

func TestRegression_PlaceChat_RepoRootDropIndexIsNotShiftedByTheDefaultCheckoutAnchor(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	gitStatus.SetRepo(workspaceID, repoID)
	gitStatus.SetDefault(repoID, workspaceID)
	gitStatus.SetBranch(workspaceID, true)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "owner", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, OwnsWorkspace: true, CreatedAt: time.Unix(0, 0)},
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, ParentID: "owner", CreatedAt: time.Unix(1, 0)},
		domain.Chat{ID: "c2", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, ParentID: "owner", CreatedAt: time.Unix(2, 0)},
	)
	nodes.Rows = []dtree.Node{
		{ID: workspaceID, Kind: dtree.KindWorkspace, ParentID: "", Order: 0},
		{ID: "c1", Kind: dtree.KindChat, ParentID: "owner", Order: 1},
		{ID: "c2", Kind: dtree.KindChat, ParentID: "owner", Order: 2},
	}

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2", tree.PlaceInput{ParentID: name("owner"), Order: index(1)})
	require.NoError(t, err)
	c1, c2 := nodeRowFor(t, nodes, "c1"), nodeRowFor(t, nodes, "c2")
	assert.Less(t, c1.Order, c2.Order, "c2 dropped after c1 must stay after c1 (got c1=%d c2=%d)", c1.Order, c2.Order)
}
