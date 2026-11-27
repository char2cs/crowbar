package tree_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

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
	folders.Saved = []dtree.Folder{{ID: "repo-folder", RepoID: "repo-A"}}
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: "home-owner", Type: domain.ChatTypeChat, WorkspaceID: "ws-home",
		OwnsWorkspace: true, CreatedAt: time.Unix(1, 0),
	})
	nodes.Rows = []dtree.Node{
		{ID: "ws-home", Kind: dtree.KindWorkspace, Order: 0},
		{ID: "locked-main", Kind: dtree.KindWorkspace, Order: 0},
		{ID: "repo-folder", Kind: dtree.KindFolder, Order: 1},
	}
}

func TestRegression_MintOwningChat_UnderARepoHeaderTakesTheRepoRootsNextSlot(t *testing.T) {
	chats, folders, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	seedRepoHeaderLevel(chats, folders, nodes, gitStatus)
	chats.NextID = "c-fork"

	chatID, err := uc.MintOwningChat(context.Background(), "", "ws-home")

	require.NoError(t, err)
	row := chatRow(t, chats, chatID)
	assert.Equal(t, "ws-home", row.ParentID, "the row is still filed under the header it was forked from")
	assert.Equal(t, 2, row.Order,
		"the repo root already holds the locked branch (0) and the repo folder (1)")
}

func TestRegression_MintOwningChat_TwoForksOffARepoHeaderTakeDistinctSlots(t *testing.T) {
	chats, folders, nodes, gitStatus, uc := newHomeUsecaseFull(t)
	seedRepoHeaderLevel(chats, folders, nodes, gitStatus)
	ctx := context.Background()

	chats.NextID = "c-first"
	first, err := uc.MintOwningChat(ctx, "", "ws-home")
	require.NoError(t, err)
	require.NoError(t, uc.AttachOwningWorkspace(ctx, first, domain.Workspace{ID: "ws-first"}))
	gitStatus.SetRepo("ws-first", "repo-A")

	chats.NextID = "c-second"
	second, err := uc.MintOwningChat(ctx, "", "ws-home")
	require.NoError(t, err)

	assert.Equal(t, 2, chatRow(t, chats, first).Order)
	assert.Equal(t, 3, chatRow(t, chats, second).Order,
		"the second fork appends after the first, not on top of it")
}
