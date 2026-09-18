package tree_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The repo's DEFAULT checkout is locked (an adopted protected branch), so it
// RendersAsBranch, and it has its own Node{Kind:workspace} anchor at the bare
// root. The sidebar draws that workspace as the repo HEADER, never as a root
// sibling — so the repo-root level the header's threads share must not count
// the default's own anchor as a member.
func TestRegression_CreateChat_RepoRootLevelNeverCountsTheDefaultCheckoutAnchor(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	gitStatus.SetRepo(workspaceID, repoID)
	gitStatus.SetDefault(repoID, workspaceID)
	gitStatus.SetBranch(workspaceID, true)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "owner", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, OwnsWorkspace: true, CreatedAt: time.Unix(0, 0)},
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, ParentID: "owner", CreatedAt: time.Unix(1, 0)},
	)
	nodes.Rows = []domain.Node{
		{ID: workspaceID, Kind: domain.NodeKindWorkspace, ParentID: "", Order: 0},
		{ID: "c1", Kind: domain.NodeKindChat, ParentID: "owner", Order: 0},
	}
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "owner", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	require.NoError(t, err)
	n := nodeRowFor(t, nodes, "c-new")
	assert.Equal(t, "owner", n.ParentID)
	assert.Equal(t, 1, n.Order, "one real sibling (c1) sits at the repo root, so the thread appends at 1")
	assert.False(t, writesTo(nodes, workspaceID), "the default checkout's own anchor is the header, never a root sibling to renumber")
	assert.False(t, writesTo(nodes, "c1"), "an already dense level is not renumbered by a create")
}

// The same phantom, on a drag: the sidebar computes a drop index over the
// rows it draws under the header ([c1, c2]); the daemon inserts at that index
// into a level that also holds the default's anchor, so "after c1" (index 1)
// lands BEFORE c1.
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
	nodes.Rows = []domain.Node{
		{ID: workspaceID, Kind: domain.NodeKindWorkspace, ParentID: "", Order: 0},
		{ID: "c1", Kind: domain.NodeKindChat, ParentID: "owner", Order: 1},
		{ID: "c2", Kind: domain.NodeKindChat, ParentID: "owner", Order: 2},
	}

	// Drop c2 "after c1": index 1 among the rendered siblings [c1, c2] minus c2.
	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2", tree.PlaceInput{ParentID: name("owner"), Order: index(1)})
	require.NoError(t, err)
	c1, c2 := nodeRowFor(t, nodes, "c1"), nodeRowFor(t, nodes, "c2")
	assert.Less(t, c1.Order, c2.Order, "c2 dropped after c1 must stay after c1 (got c1=%d c2=%d)", c1.Order, c2.Order)
}
