package tree_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// seedRepoHeaderLevel builds one repo's ROOT level as the sidebar draws it:
// the default checkout "ws-home" (the repo header row, never a member of the
// level it heads) with its own owning chat, a locked branch drawn from its
// workspace anchor at slot 0, and a repo folder at slot 1.
func seedRepoHeaderLevel(
	chats *mocks.AgentChatPlacements,
	folders *mocks.FolderStore,
	nodes *mocks.NodePlacements,
	gitStatus *mocks.AgentWorkspaceGitStatus,
) {
	gitStatus.SetRepo("ws-home", "repo-A")
	gitStatus.SetDefault("repo-A", "ws-home")
	gitStatus.SetRepo("locked-main", "repo-A")
	gitStatus.SetBranch("locked-main", true)
	folders.Saved = []domain.Folder{{ID: "repo-folder", RepoID: "repo-A"}}
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: "home-owner", Type: domain.ChatTypeChat, WorkspaceID: "ws-home",
		OwnsWorkspace: true, CreatedAt: time.Unix(1, 0),
	})
	nodes.Rows = []domain.Node{
		{ID: "ws-home", Kind: domain.NodeKindWorkspace, Order: 0},
		{ID: "locked-main", Kind: domain.NodeKindWorkspace, Order: 0},
		{ID: "repo-folder", Kind: domain.NodeKindFolder, Order: 1},
	}
}

// TestRegression_MintOwningChat_UnderARepoHeaderTakesTheRepoRootsNextSlot is
// the repo-header half of "one canonical sibling scope per level".
//
// A row forked from the repo HEADER is filed under the default checkout's own
// id, and that id names the repo ROOT level (levelAliases folds a default
// checkout and its owning chat onto ""), which is where the sidebar draws it —
// beside the locked default branch and the repo's folders. placeOwningRow
// counted only the chats whose ParentID was literally that checkout's id, an
// independently-dense scope of its own, so the first fork off a repo header
// always claimed order 0 while the rows it renders among were already numbered
// 0..N. Caught live: the identical gesture put the new row ABOVE the locked
// `main` on one repo and BELOW it on another, purely on whether `main`'s own
// order had ever been written.
func TestRegression_MintOwningChat_UnderARepoHeaderTakesTheRepoRootsNextSlot(t *testing.T) {
	chats, folders, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	seedRepoHeaderLevel(chats, folders, nodes, gitStatus)
	chats.NextID = "c-fork"

	chatID, err := uc.MintOwningChat(context.Background(), "ws-home")

	require.NoError(t, err)
	row := chatRow(t, chats, chatID)
	assert.Equal(t, "ws-home", row.ParentID, "the row is still filed under the header it was forked from")
	assert.Equal(t, 2, row.Order,
		"the repo root already holds the locked branch (0) and the repo folder (1)")
}

// The same level, counted a second time: two forks off one repo header must
// not both claim the slot after the level, and neither may collide with the
// rows already in it.
func TestRegression_MintOwningChat_TwoForksOffARepoHeaderTakeDistinctSlots(t *testing.T) {
	chats, folders, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	seedRepoHeaderLevel(chats, folders, nodes, gitStatus)
	ctx := context.Background()

	chats.NextID = "c-first"
	first, err := uc.MintOwningChat(ctx, "ws-home")
	require.NoError(t, err)
	require.NoError(t, uc.AttachOwningWorkspace(ctx, first, domain.Workspace{ID: "ws-first"}))
	gitStatus.SetRepo("ws-first", "repo-A")

	chats.NextID = "c-second"
	second, err := uc.MintOwningChat(ctx, "ws-home")
	require.NoError(t, err)

	assert.Equal(t, 2, chatRow(t, chats, first).Order)
	assert.Equal(t, 3, chatRow(t, chats, second).Order,
		"the second fork appends after the first, not on top of it")
}
