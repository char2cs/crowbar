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

// Two locked branches at order 0: the sidebar draws the OLDER first (a branch
// row carries its workspace's createdAt), so a chat dropped "after the first
// branch" is index 1. An anchor read off its Node alone carried a zero
// CreatedAt and tied by id — the lexically earlier "aa" came first on the
// daemon, and the chat landed after the wrong branch.
func TestRegression_PlaceChat_TiedAnchorsSortByWorkspaceCreatedAtNotID(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	gitStatus.SetRepo(workspaceID, repoID)
	gitStatus.SetDefault(repoID, workspaceID)
	for id, at := range map[string]time.Time{"zz-older": time.Unix(100, 0), "aa-newer": time.Unix(200, 0)} {
		gitStatus.SetRepo(id, repoID)
		gitStatus.SetBranch(id, true)
		gitStatus.SetCreatedAt(id, at)
		nodes.Rows = append(nodes.Rows, domain.Node{ID: id, Kind: domain.NodeKindWorkspace, ParentID: "", Order: 0})
	}
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, CreatedAt: time.Unix(300, 0)},
	)
	nodes.Rows = append(nodes.Rows, domain.Node{ID: "c1", Kind: domain.NodeKindChat, ParentID: "", Order: 2})

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1", tree.PlaceInput{ParentID: name(""), Order: index(1)})
	require.NoError(t, err)
	older, newer, c1 := nodeRowFor(t, nodes, "zz-older"), nodeRowFor(t, nodes, "aa-newer"), nodeRowFor(t, nodes, "c1")
	assert.Equal(t, 0, older.Order, "the branch created first is drawn first")
	assert.Equal(t, 1, c1.Order)
	assert.Equal(t, 2, newer.Order)
}

// A locked branch from before Node rows existed has none, so the repo-root
// densify never counted it: a drop index computed over the drawn rows named a
// different slot on the daemon.
func TestRegression_PlaceChat_CountsALegacyNodelessLockedBranch(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	gitStatus.SetRepo(workspaceID, repoID)
	gitStatus.SetDefault(repoID, workspaceID)
	gitStatus.SetRepo("legacy-locked", repoID)
	gitStatus.SetBranch("legacy-locked", true)
	gitStatus.SetCreatedAt("legacy-locked", time.Unix(100, 0))
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, CreatedAt: time.Unix(300, 0)},
		domain.Chat{ID: "c2", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, CreatedAt: time.Unix(400, 0)},
	)
	nodes.Rows = []domain.Node{
		{ID: "c1", Kind: domain.NodeKindChat, ParentID: "", Order: 1},
		{ID: "c2", Kind: domain.NodeKindChat, ParentID: "", Order: 2},
	}

	// Drawn [legacy-locked, c1, c2]; drop c2 right after the branch.
	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2", tree.PlaceInput{ParentID: name(""), Order: index(1)})
	require.NoError(t, err)
	anchor := nodeRowFor(t, nodes, "legacy-locked")
	assert.Equal(t, domain.NodeKindWorkspace, anchor.Kind, "the first write under the level mints the legacy anchor's Node")
	assert.Equal(t, 0, anchor.Order)
	assert.Equal(t, 1, nodeRowFor(t, nodes, "c2").Order)
	assert.Equal(t, 2, nodeRowFor(t, nodes, "c1").Order)
}
