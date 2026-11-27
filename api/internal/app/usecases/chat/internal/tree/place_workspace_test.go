package tree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

func TestPlaceWorkspace_MovesAnOrdinaryUnlockedForkJustLikeALockedBranch(t *testing.T) {
	_, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	gitStatus.SetRepo("fork-1", repoID)
	gitStatus.SetBranch("fork-1", false)

	_, _, err := uc.PlaceWorkspace(context.Background(), "fork-1", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err)

	n := nodeRowFor(t, nodes, "fork-1")
	assert.Equal(t, dtree.KindWorkspace, n.Kind)
}

func TestRegression_PlaceWorkspace_AnOrdinaryForkWritesThroughItsOwningChat(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "branch-1", Type: domain.ChatTypeBranch, OwnsWorkspace: true, WorkspaceID: "ws-branch-1"},
		domain.Chat{ID: "fork-chat", Type: domain.ChatTypeChat, OwnsWorkspace: true, WorkspaceID: "ws-fork", ParentID: "branch-1"},
		domain.Chat{ID: "sibling-chat", Type: domain.ChatTypeChat, OwnsWorkspace: true, WorkspaceID: "ws-sibling", ParentID: "branch-1"},
	)
	nodes.Rows = append(nodes.Rows,
		dtree.Node{ID: "fork-chat", Kind: dtree.KindChat, ParentID: "branch-1", Order: 0},
		dtree.Node{ID: "sibling-chat", Kind: dtree.KindChat, ParentID: "branch-1", Order: 1},
	)
	gitStatus.SetRepo("ws-branch-1", repoID)
	gitStatus.SetRepo("ws-fork", repoID)
	gitStatus.SetBranch("ws-fork", false)

	_, _, err := uc.PlaceWorkspace(context.Background(), "ws-fork", tree.PlaceInput{Order: index(1)})
	require.NoError(t, err)

	n := nodeRowFor(t, nodes, "fork-chat")
	assert.Equal(t, "branch-1", n.ParentID, "the fork's real container must survive a plain reorder")
	assert.Equal(t, 1, n.Order, "the requested order must land on the row the panel actually reads")
	assert.Equal(t, 0, nodeRowFor(t, nodes, "sibling-chat").Order, "pushed up to make room")
	for _, row := range nodes.Rows {
		assert.NotEqual(t, "ws-fork", row.ID,
			"the workspace's own abandoned anchor row must never be written instead")
	}
}

func TestRegression_PlaceWorkspace_AnOrdinaryForkReorderedPastASiblingFork(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "branch-1", Type: domain.ChatTypeBranch, OwnsWorkspace: true, WorkspaceID: "ws-branch-1"},
		domain.Chat{ID: "fork-a", Type: domain.ChatTypeBranch, OwnsWorkspace: true, WorkspaceID: "ws-fork-a", ParentID: "branch-1"},
		domain.Chat{ID: "fork-b", Type: domain.ChatTypeBranch, OwnsWorkspace: true, WorkspaceID: "ws-fork-b", ParentID: "branch-1"},
	)
	gitStatus.SetRepo("ws-branch-1", repoID)
	gitStatus.SetBranch("ws-branch-1", true)
	gitStatus.SetRepo("ws-fork-a", repoID)
	gitStatus.SetBranch("ws-fork-a", false)
	gitStatus.SetRepo("ws-fork-b", repoID)
	gitStatus.SetBranch("ws-fork-b", false)

	_, _, err := uc.PlaceWorkspace(context.Background(), "ws-fork-a", tree.PlaceInput{Order: index(1)})
	require.NoError(t, err)

	n := nodeRowFor(t, nodes, "fork-a")
	assert.Equal(t, "branch-1", n.ParentID, "the fork's real container must survive a plain reorder")
	assert.Equal(t, 1, n.Order, "the requested order must land on the Node the panel actually reads, not Chat.Order")
}

func TestPlaceWorkspace_RefusesTheProjectHomeWorkspace(t *testing.T) {
	_, _, _, gitStatus, uc := newWorkspacePlacementUsecase(t)
	gitStatus.SetRepo("home-ws", "")

	_, _, err := uc.PlaceWorkspace(context.Background(), "home-ws", tree.PlaceInput{Order: index(0)})

	require.ErrorIs(t, err, problem.ErrNotFound)
}

func TestPlaceWorkspace_RefusesAnUnknownWorkspaceID(t *testing.T) {
	_, _, _, _, uc := newWorkspacePlacementUsecase(t)

	_, _, err := uc.PlaceWorkspace(context.Background(), "no-such-workspace", tree.PlaceInput{Order: index(0)})

	require.ErrorIs(t, err, problem.ErrNotFound)
}

func TestPlaceWorkspace_MintsNodeOnFirstTouch(t *testing.T) {
	_, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)

	_, _, err := uc.PlaceWorkspace(context.Background(), "branch-1", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err, "a locked branch with no Node row must mint one, not silently do nothing")

	n := nodeRowFor(t, nodes, "branch-1")
	assert.Equal(t, dtree.KindWorkspace, n.Kind)
	assert.Equal(t, "", n.ParentID)
	assert.Equal(t, 0, n.Order)
}

func TestPlaceWorkspace_MovesIntoAFolderWithinItsOwnRepo(t *testing.T) {
	_, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	seedFolder(t, uc, "docs", "")
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)

	placed, _, err := uc.PlaceWorkspace(context.Background(), "branch-1", tree.PlaceInput{ParentID: name("docs")})
	require.NoError(t, err)

	assert.Equal(t, "docs", placed.ParentID)
	assert.Equal(t, "docs", nodeRowFor(t, nodes, "branch-1").ParentID)
}

func TestPlaceWorkspace_RefusesACrossRepoContainer(t *testing.T) {
	chats, _, _, gitStatus, uc := newWorkspacePlacementUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: "f-other-repo", Type: domain.ChatTypeFolder, RepoID: "repo-2",
	})
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)

	_, _, err := uc.PlaceWorkspace(
		context.Background(), "branch-1", tree.PlaceInput{ParentID: name("f-other-repo")})

	require.ErrorIs(t, err, tree.ErrCrossContext)
}

func TestPlaceWorkspace_MovingToTheBareRepoRootNeverCrossesContext(t *testing.T) {
	_, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	seedFolder(t, uc, "docs", "")
	nodes.Rows = append(nodes.Rows, dtree.Node{
		ID: "branch-1", Kind: dtree.KindWorkspace, ParentID: "docs", Order: 0,
	})
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)

	placed, _, err := uc.PlaceWorkspace(context.Background(), "branch-1", tree.PlaceInput{ParentID: name("")})
	require.NoError(t, err)

	assert.Equal(t, "", placed.ParentID)
	assert.Equal(t, "", nodeRowFor(t, nodes, "branch-1").ParentID)
}

func TestPlaceWorkspace_SecondMoveWritesPlacementNotCreate(t *testing.T) {
	_, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	seedFolder(t, uc, "folder-a", "")
	seedFolder(t, uc, "folder-b", "")
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)
	_, _, err := uc.PlaceWorkspace(context.Background(), "branch-1", tree.PlaceInput{ParentID: name("folder-a")})
	require.NoError(t, err)

	_, _, err = uc.PlaceWorkspace(context.Background(), "branch-1", tree.PlaceInput{ParentID: name("folder-b")})
	require.NoError(t, err)

	assert.Equal(t, "folder-b", nodeRowFor(t, nodes, "branch-1").ParentID)
	require.NotEmpty(t, nodes.Placed)
	assert.Equal(t, "branch-1", nodes.Placed[len(nodes.Placed)-1].ID)
}

func TestPlaceWorkspace_RefusesWorkingSubtree(t *testing.T) {
	chats, work, _, gitStatus, uc := newWorkspacePlacementUsecaseWithWork(t)
	seedFolder(t, uc, "docs", "")
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: "child-1", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, ParentID: "branch-1",
	})
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)
	work.Set("child-1", true)

	_, _, err := uc.PlaceWorkspace(context.Background(), "branch-1", tree.PlaceInput{ParentID: name("docs")})

	assert.ErrorIs(t, err, tree.ErrSubtreeWorking)
}

func TestRegression_PlaceWorkspace_RefusedWhenItWouldSplitAForkChain(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	seedForkUnderLockedBranch(t, chats, nodes, gitStatus)
	seedFolder(t, uc, "spikes", "")

	_, _, err := uc.PlaceWorkspace(
		context.Background(), "ws-fork", tree.PlaceInput{ParentID: name("spikes")})

	require.ErrorIs(t, err, tree.ErrForkChain)
	assert.Contains(t, err.Error(), "fork parent",
		"the refusal has to say what is wrong, not just refuse")
	assert.Equal(t, "branch-1", nodeRowFor(t, nodes, "fork-chat").ParentID,
		"a refused placement must never move the row it refused")
}

func TestPlaceWorkspace_AForkMayBeFiledInAFolderInsideItsOwnForkParent(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	seedForkUnderLockedBranch(t, chats, nodes, gitStatus)
	seedFolder(t, uc, "spikes", "branch-1")

	_, _, err := uc.PlaceWorkspace(
		context.Background(), "ws-fork", tree.PlaceInput{ParentID: name("spikes")})

	require.NoError(t, err)
	assert.Equal(t, "spikes", nodeRowFor(t, nodes, "fork-chat").ParentID)
}

func TestPlaceWorkspace_AForkCutOffTheReposOwnCheckoutStaysFreeAtTheRoot(t *testing.T) {
	_, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	seedFolder(t, uc, "spikes", "")
	gitStatus.SetRepo("branch-1", repoID)
	gitStatus.SetBranch("branch-1", true)
	gitStatus.SetForkParent("branch-1", "")

	_, _, err := uc.PlaceWorkspace(
		context.Background(), "branch-1", tree.PlaceInput{ParentID: name("spikes")})

	require.NoError(t, err)
	assert.Equal(t, "spikes", nodeRowFor(t, nodes, "branch-1").ParentID)
}

func TestPlaceWorkspace_AnAlreadySplitForkCanStillBeReordered(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	seedFolder(t, uc, "spikes", "")
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "fork-chat", Type: domain.ChatTypeChat, OwnsWorkspace: true, WorkspaceID: "ws-fork", ParentID: "spikes"},
		domain.Chat{ID: "sibling", Type: domain.ChatTypeChat, WorkspaceID: workspaceID, ParentID: "spikes"},
	)
	nodes.Rows = append(nodes.Rows,
		dtree.Node{ID: "fork-chat", Kind: dtree.KindChat, ParentID: "spikes", Order: 0},
		dtree.Node{ID: "sibling", Kind: dtree.KindChat, ParentID: "spikes", Order: 1},
	)
	gitStatus.SetRepo("ws-fork", repoID)
	gitStatus.SetBranch("ws-fork", false)
	gitStatus.SetForkParent("ws-fork", "ws-branch-1")

	_, _, err := uc.PlaceWorkspace(context.Background(), "ws-fork", tree.PlaceInput{Order: index(1)})

	require.NoError(t, err)
	assert.Equal(t, "spikes", nodeRowFor(t, nodes, "fork-chat").ParentID)
	assert.Equal(t, 1, nodeRowFor(t, nodes, "fork-chat").Order)
}

func seedForkUnderLockedBranch(
	t *testing.T,
	chats *mocks.AgentChatPlacements,
	nodes *mocks.NodePlacements,
	gitStatus *mocks.AgentWorkspaceGitStatus,
) {
	t.Helper()
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "branch-1", Type: domain.ChatTypeBranch, OwnsWorkspace: true, WorkspaceID: "ws-branch-1"},
		domain.Chat{ID: "fork-chat", Type: domain.ChatTypeChat, OwnsWorkspace: true, WorkspaceID: "ws-fork", ParentID: "branch-1"},
	)
	nodes.Rows = append(nodes.Rows,
		dtree.Node{ID: "fork-chat", Kind: dtree.KindChat, ParentID: "branch-1", Order: 0})
	gitStatus.SetRepo("ws-branch-1", repoID)
	gitStatus.SetBranch("ws-branch-1", true)
	gitStatus.SetRepo("ws-fork", repoID)
	gitStatus.SetBranch("ws-fork", false)
	gitStatus.SetForkParent("ws-fork", "ws-branch-1")
}

func newWorkspacePlacementUsecase(
	t *testing.T,
) (
	*mocks.AgentChatPlacements,
	*mocks.FolderStore,
	*mocks.NodePlacements,
	*mocks.AgentWorkspaceGitStatus,
	tree.Usecase,
) {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	gitStatus := mocks.NewAgentWorkspaceGitStatus()
	uc := tree.New(chats, chats, newBusyChats(), gitStatus,
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats), folders, nodes)
	return chats, folders, nodes, gitStatus, uc
}

func newWorkspacePlacementUsecaseWithWork(
	t *testing.T,
) (
	*mocks.AgentChatPlacements,
	*busyChats,
	*mocks.NodePlacements,
	*mocks.AgentWorkspaceGitStatus,
	tree.Usecase,
) {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	gitStatus := mocks.NewAgentWorkspaceGitStatus()
	work := newBusyChats()
	uc := tree.New(chats, chats, work, gitStatus,
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats), folders, nodes)
	return chats, work, nodes, gitStatus, uc
}
