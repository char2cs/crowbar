package tree_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

type busyChats struct {
	mu  sync.Mutex
	ids map[string]bool
}

func newBusyChats() *busyChats { return &busyChats{ids: map[string]bool{}} }

func (b *busyChats) Set(chatID string, busy bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ids[chatID] = busy
}

func (b *busyChats) ChatBusy(_ context.Context, chatID string) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ids[chatID], nil
}

func newUsecaseWithWork(
	t *testing.T,
) (*mocks.AgentChatPlacements, tree.Usecase, *busyChats) {
	t.Helper()
	chats, _, _, uc, work := newUsecaseWithStores(t)
	return chats, uc, work
}

func newUsecaseWithStores(
	t *testing.T,
) (*mocks.AgentChatPlacements, *mocks.FolderStore, *mocks.NodePlacements, tree.Usecase, *busyChats) {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	chats.Nodes = nodes
	work := newBusyChats()
	workspaceGitStatus := mocks.NewAgentWorkspaceGitStatus()
	workspaceGitStatus.SetRepo(workspaceID, repoID)
	uc := tree.New(chats, chats, work, workspaceGitStatus,
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats),
		folders, nodes)
	return chats, folders, nodes, uc, work
}

func seedFolderTree(
	t *testing.T,
	chats *mocks.AgentChatPlacements,
	uc tree.Usecase,
) {
	t.Helper()
	ctx := context.Background()
	_, _, err := uc.Create(ctx, tree.CreateInput{ID: "root", RepoID: repoID, Name: "root"})
	require.NoError(t, err)
	_, _, err = uc.Create(ctx, tree.CreateInput{ID: "other", RepoID: repoID, Name: "other"})
	require.NoError(t, err)
	seedThread(chats, "child-1", "root", 1)
	seedThread(chats, "child-2", "root", 2)
}

func TestMove_RefusesWorkingSubtree(t *testing.T) {
	chats, uc, work := newUsecaseWithWork(t)
	seedFolderTree(t, chats, uc)
	work.Set("child-1", true)

	_, _, err := uc.Move(context.Background(), "root", tree.MoveInput{ParentID: name("other")})
	assert.ErrorIs(t, err, tree.ErrSubtreeWorking)
}

func TestMove_TakesWholeSubtree(t *testing.T) {
	chats, uc, _ := newUsecaseWithWork(t)
	seedFolderTree(t, chats, uc)

	placed, _, err := uc.Move(context.Background(), "root", tree.MoveInput{ParentID: name("other")})
	require.NoError(t, err)

	assert.Equal(t, "other", placed.ParentID)
	assert.Equal(t, "root", chatRow(t, chats, "child-1").ParentID)
	assert.Equal(t, "root", chatRow(t, chats, "child-2").ParentID)
}

func TestMove_RefusesAMoveAcrossRepos(t *testing.T) {
	chats, uc, _ := newUsecaseWithWork(t)
	seedFolderTree(t, chats, uc)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "other-repo-folder", Type: domain.ChatTypeFolder, RepoID: "repo-2"},
	)

	_, _, err := uc.Move(context.Background(), "root", tree.MoveInput{ParentID: name("other-repo-folder")})
	assert.ErrorIs(t, err, tree.ErrCrossContext)
}

func TestMove_RefusesRootToBranchContext(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	seedFolderTree(t, chats, uc)
	seedChat(chats, "branch-1", 1)
	nodes.Rows = append(nodes.Rows, dtree.Node{ID: "branch-1", Kind: dtree.KindWorkspace})

	_, _, err := uc.Move(context.Background(), "root", tree.MoveInput{ParentID: name("branch-1")})
	assert.ErrorIs(t, err, tree.ErrCrossContext)
}

func TestMove_RefusesBranchToDifferentBranchContext(t *testing.T) {
	chats := mocks.NewAgentChatPlacements()
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "branch-1", Type: domain.ChatTypeChat, WorkspaceID: "ws-1"},
		domain.Chat{ID: "branch-2", Type: domain.ChatTypeChat, WorkspaceID: "ws-2"},
	)
	nodes := mocks.NewNodePlacements()
	nodes.Rows = append(nodes.Rows,
		dtree.Node{ID: "branch-1", Kind: dtree.KindWorkspace},
		dtree.Node{ID: "branch-2", Kind: dtree.KindWorkspace},
	)
	gitStatus := mocks.NewAgentWorkspaceGitStatus()
	gitStatus.SetRepo("ws-1", repoID)
	gitStatus.SetRepo("ws-2", repoID)
	uc2 := tree.New(chats, chats, newBusyChats(), gitStatus,
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats),
		mocks.NewFolderStore(), nodes)
	underBranch1, _, err := uc2.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "branch-1", Name: "notes",
	})
	require.NoError(t, err)

	_, _, err = uc2.Move(context.Background(), underBranch1.ID, tree.MoveInput{ParentID: name("branch-2")})
	assert.ErrorIs(t, err, tree.ErrCrossContext)
}

func TestMove_AllowsAMoveWithinTheSameBranchContext(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	seedChat(chats, "branch-1", 1)
	nodes.Rows = append(nodes.Rows, dtree.Node{ID: "branch-1", Kind: dtree.KindWorkspace})
	underBranch1, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "branch-1", Name: "notes",
	})
	require.NoError(t, err)
	nestedDeeper, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "branch-1", Name: "deeper",
	})
	require.NoError(t, err)

	placed, _, err := uc.Move(context.Background(), underBranch1.ID, tree.MoveInput{
		ParentID: name(nestedDeeper.ID),
	})
	require.NoError(t, err)
	assert.Equal(t, nestedDeeper.ID, placed.ParentID)
}

func TestMove_AllowsAFolderIntoAPlainChatSiblingAtTheSameRoot(t *testing.T) {
	chats, uc, _ := newUsecaseWithWork(t)
	seedChat(chats, "owner", 1)
	seedChat(chats, "sibling", 2)
	folder, _, err := uc.Create(context.Background(), tree.CreateInput{RepoID: repoID, Name: "folder"})
	require.NoError(t, err)

	placed, _, err := uc.Move(context.Background(), folder.ID, tree.MoveInput{ParentID: name("sibling")})
	require.NoError(t, err)
	assert.Equal(t, "sibling", placed.ParentID)
}

func TestMove_DensifiesAgainstASiblingFolderSharingAnUnNodedBranchContainer(t *testing.T) {
	chats, uc, _ := newUsecaseWithWork(t)
	seedChat(chats, "branch-1", 1)
	first, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "branch-1", Name: "first",
	})
	require.NoError(t, err)
	require.Equal(t, 0, first.Order)

	second, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "branch-1", Name: "second",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, second.Order, "the second folder must see the first as an occupied slot")

	placed, shifted, err := uc.Move(context.Background(), second.ID, tree.MoveInput{Order: index(0)})
	require.NoError(t, err)
	assert.Equal(t, 0, placed.Order)
	require.Len(t, shifted, 1)
	assert.Equal(t, first.ID, shifted[0].ID)
	assert.Equal(t, 1, shifted[0].Order, "the sibling folder correctly shifted too")
}

func TestPlaceChat_NeverDensifiesAgainstAnyRepoPhantom(t *testing.T) {
	chats, nodes, gitStatus, uc := newHomeUsecaseWithGitStatus(t)
	gitStatus.SetRepo(workspaceID, repoID)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: workspaceID})
	nodes.Rows = []dtree.Node{
		{ID: "repo-1", Kind: dtree.KindRepo, Order: 0},
		{ID: "repo-2", Kind: dtree.KindRepo, Order: 1},
	}

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, 0, nodeRowFor(t, nodes, "repo-1").Order, "no repo phantom is EVER a sibling inside a repo's own tree")
	assert.Equal(t, 1, nodeRowFor(t, nodes, "repo-2").Order)
	for _, w := range nodes.Ordered {
		assert.NotEqual(t, "repo-1", w.ID, "a repo phantom must never be WRITTEN by a repo-internal densify")
		assert.NotEqual(t, "repo-2", w.ID)
	}
	for _, w := range nodes.Placed {
		assert.NotEqual(t, "repo-1", w.ID)
		assert.NotEqual(t, "repo-2", w.ID)
	}
}

func TestPlaceChat_RefusesWorkingSubtree(t *testing.T) {
	chats, uc, work := newUsecaseWithWork(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)
	seedChat(chats, "other", 3)
	work.Set("c2", true)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		tree.PlaceInput{ParentID: name("other")})
	assert.ErrorIs(t, err, tree.ErrSubtreeWorking)
}

func TestPlaceChat_SecondMoveOfAChatUnreachableThroughItsBranchParent(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "branch-1", Type: domain.ChatTypeBranch, WorkspaceID: "ws-branch-1"},
		domain.Chat{ID: "fork-chat", Type: domain.ChatTypeChat, WorkspaceID: "ws-fork", ParentID: "branch-1"},
	)
	gitStatus.SetRepo("ws-fork", repoID)
	gitStatus.SetRepo("ws-branch-1", repoID)
	nodes.Rows = append(nodes.Rows, dtree.Node{
		ID: "fork-chat", Kind: dtree.KindChat, ParentID: "branch-1", Order: 0,
	})

	_, _, err := uc.PlaceChat(context.Background(), "ws-fork", "fork-chat", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err,
		"a chat already Node-backed must never be re-Created just because this walk could not reach it")

	assert.Equal(t, "branch-1", nodeRowFor(t, nodes, "fork-chat").ParentID)
}

func TestRegression_PlaceChat_DensifiesAgainstOtherForksUnderTheSameLockedBranch(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "branch-1", Type: domain.ChatTypeBranch, WorkspaceID: "ws-branch-1"},
		domain.Chat{ID: "fork-a", Type: domain.ChatTypeChat, WorkspaceID: "ws-fork-a", ParentID: "branch-1"},
		domain.Chat{ID: "fork-b", Type: domain.ChatTypeChat, WorkspaceID: "ws-fork-b", ParentID: "branch-1"},
		domain.Chat{ID: "fork-c", Type: domain.ChatTypeChat, WorkspaceID: "ws-fork-c", ParentID: "branch-1"},
	)
	gitStatus.SetRepo("ws-fork-a", repoID)
	gitStatus.SetRepo("ws-fork-b", repoID)
	gitStatus.SetRepo("ws-fork-c", repoID)
	gitStatus.SetRepo("ws-branch-1", repoID)
	nodes.Rows = []dtree.Node{
		{ID: "fork-a", Kind: dtree.KindChat, ParentID: "branch-1", Order: 0},
		{ID: "fork-b", Kind: dtree.KindChat, ParentID: "branch-1", Order: 1},
		{ID: "fork-c", Kind: dtree.KindChat, ParentID: "branch-1", Order: 2},
	}

	_, _, err := uc.PlaceChat(context.Background(), "ws-fork-c", "fork-c", tree.PlaceInput{Order: index(1)})
	require.NoError(t, err)

	assert.Equal(t, 1, nodeRowFor(t, nodes, "fork-c").Order,
		"the requested index must survive, not collapse to 0 for lack of a known container size")
	assert.Equal(t, 0, nodeRowFor(t, nodes, "fork-a").Order, "untouched: still first")
	assert.Equal(t, 2, nodeRowFor(t, nodes, "fork-b").Order, "pushed down to make room")
}
