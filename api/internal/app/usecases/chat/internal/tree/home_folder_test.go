package tree_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

const homeWorkspaceID = "home-ws-1"

func newHomeUsecase(
	t *testing.T,
) (
	*mocks.AgentChatPlacements,
	*mocks.FolderStore,
	*mocks.NodePlacements,
	tree.Usecase,
) {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	gitStatus := mocks.NewAgentWorkspaceGitStatus()
	uc := tree.New(chats, chats, newBusyChats(), gitStatus,
		mocks.NewAgentWorkspaceReaper(), mocks.NewAgentWorkspaceHolders(chats), folders, nodes)
	return chats, folders, nodes, uc
}

func newHomeUsecaseWithGitStatus(
	t *testing.T,
) (
	*mocks.AgentChatPlacements,
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
	return chats, nodes, gitStatus, uc
}

func nodeRowFor(
	t *testing.T,
	nodes *mocks.NodePlacements,
	id string,
) dtree.Node {
	t.Helper()
	for _, n := range nodes.Rows {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("nodeRowFor: no Node row for %s", id)
	return dtree.Node{}
}

func TestCreate_Home_MintsFolderPlusNode(t *testing.T) {
	chats, folders, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()

	created, shifted, err := uc.Create(ctx, tree.CreateInput{Name: "docs"})
	require.NoError(t, err)

	assert.Equal(t, "docs", created.Title)
	assert.Equal(t, domain.ChatTypeFolder, created.Type)
	assert.Equal(t, "", created.RepoID)
	assert.NotEmpty(t, created.ID)
	assert.Empty(t, shifted)

	require.Len(t, folders.Saved, 1)
	assert.Equal(t, created.ID, folders.Saved[0].ID)
	assert.Equal(t, "docs", folders.Saved[0].Name)
	assert.Equal(t, "", folders.Saved[0].RepoID)

	require.Len(t, nodes.Rows, 1)
	assert.Equal(t, created.ID, nodes.Rows[0].ID)
	assert.Equal(t, dtree.KindFolder, nodes.Rows[0].Kind)

	assert.Empty(t, chats.Rows, "a home folder is never a Chat row")
}

func TestCreate_Home_TrimsAndRefusesABlankName(t *testing.T) {
	_, folders, _, uc := newHomeUsecase(t)
	ctx := context.Background()

	created, _, err := uc.Create(ctx, tree.CreateInput{Name: "  docs  "})
	require.NoError(t, err)
	assert.Equal(t, "docs", created.Title)

	_, _, err = uc.Create(ctx, tree.CreateInput{Name: "   "})
	assert.ErrorIs(t, err, tree.ErrNameRequired)
	assert.Len(t, folders.Saved, 1, "the refused second create mints nothing beyond the first")
}

func TestCreate_Home_RefusesUnderARepoScopedFolder(t *testing.T) {
	chats, _, _, uc := newHomeUsecase(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "repo-folder", Type: domain.ChatTypeFolder, RepoID: repoID})

	_, _, err := uc.Create(context.Background(), tree.CreateInput{ParentID: "repo-folder", Name: "docs"})
	assert.ErrorIs(t, err, tree.ErrCrossContext)
}

func TestCreate_Home_DiscardsOnPlacementFailure(t *testing.T) {
	_, folders, nodes, uc := newHomeUsecase(t)
	nodes.CreateErr = errors.New("disk full")

	_, _, err := uc.Create(context.Background(), tree.CreateInput{Name: "docs"})
	require.Error(t, err)
	assert.Empty(t, folders.Saved, "the folder row is taken back out when placement fails")
}

func TestRename_Home_UpdatesFolderNameOnly(t *testing.T) {
	chats, folders, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, tree.CreateInput{Name: "docs"})
	require.NoError(t, err)

	renamed, err := uc.Rename(ctx, created.ID, "  guides  ")
	require.NoError(t, err)
	assert.Equal(t, "guides", renamed.Title)

	require.Len(t, folders.Saved, 1)
	assert.Equal(t, "guides", folders.Saved[0].Name)
	assert.Equal(t, created.Order, nodeRowFor(t, nodes, created.ID).Order, "position untouched by a rename")
	assert.Empty(t, chats.Rows, "renaming a home folder never touches the chat aggregate")
}

func TestRename_Home_UnknownIDIsNotFound(t *testing.T) {
	_, _, _, uc := newHomeUsecase(t)
	_, err := uc.Rename(context.Background(), "nope", "new name")
	assert.Error(t, err)
}

func TestMove_Home_ReparentsViaNode(t *testing.T) {
	chats, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	a, _, err := uc.Create(ctx, tree.CreateInput{ID: "a", Name: "a"})
	require.NoError(t, err)
	b, _, err := uc.Create(ctx, tree.CreateInput{ID: "b", Name: "b"})
	require.NoError(t, err)

	placed, _, err := uc.Move(ctx, a.ID, tree.MoveInput{ParentID: name(b.ID)})
	require.NoError(t, err)
	assert.Equal(t, b.ID, placed.ParentID)

	n := nodeRowFor(t, nodes, "a")
	assert.Equal(t, "b", n.ParentID)
	assert.Empty(t, chats.Placed, "a home move never writes the chat aggregate")
	assert.Empty(t, chats.Ordered, "a home move never writes the chat aggregate")
}

func TestMove_Home_ANodeLookupFailureIsReturned(t *testing.T) {
	_, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	a, _, err := uc.Create(ctx, tree.CreateInput{ID: "a", Name: "a"})
	require.NoError(t, err)
	cause := errors.New("node read down")
	nodes.GetErr = cause

	_, _, err = uc.Move(ctx, a.ID, tree.MoveInput{Order: index(0)})

	require.ErrorIs(t, err, cause)
}

func TestMove_Home_RefusesFilingUnderARepo(t *testing.T) {
	_, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	nodes.Rows = append(nodes.Rows, dtree.Node{ID: "repo-1", Kind: dtree.KindRepo, Order: 0})
	folder, _, err := uc.Create(ctx, tree.CreateInput{Name: "docs"})
	require.NoError(t, err)

	_, _, err = uc.Move(ctx, folder.ID, tree.MoveInput{ParentID: name("repo-1")})
	assert.ErrorIs(t, err, tree.ErrNotAContainer)
}

func TestPlaceChat_Home_RefusesFilingUnderARepo(t *testing.T) {
	chats, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	nodes.Rows = append(nodes.Rows, dtree.Node{ID: "repo-1", Kind: dtree.KindRepo, Order: 0})
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID})

	_, _, err := uc.PlaceChat(ctx, homeWorkspaceID, "c1", tree.PlaceInput{ParentID: name("repo-1")})
	assert.ErrorIs(t, err, tree.ErrNotAContainer)
}

func TestRegression_PlaceChat_Home_DoesNotCorruptAnotherProjectsRepoSharingTheBareRoot(t *testing.T) {
	chats, nodes, gitStatus, uc := newHomeUsecaseWithGitStatus(t)
	ctx := context.Background()
	gitStatus.SetHomeRepoMembers(homeWorkspaceID, "repo-A")
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID})
	nodes.Rows = []dtree.Node{
		{ID: "repo-A", Kind: dtree.KindRepo, Order: 0},
		{ID: "repo-B", Kind: dtree.KindRepo, Order: 1},
	}

	_, _, err := uc.PlaceChat(ctx, homeWorkspaceID, "c1", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, 1, nodeRowFor(t, nodes, "repo-B").Order,
		"another project's repo sharing the bare root must be untouched")
	for _, w := range nodes.Ordered {
		assert.NotEqual(t, "repo-B", w.ID, "another project's repo must never be WRITTEN")
	}
	for _, w := range nodes.Placed {
		assert.NotEqual(t, "repo-B", w.ID, "another project's repo must never be WRITTEN")
	}
	assert.Equal(t, 1, nodeRowFor(t, nodes, "repo-A").Order,
		"this project's own repo still shifts correctly")
	assert.Equal(t, 0, nodeRowFor(t, nodes, "c1").Order)
}

func TestMove_Home_DensifiesAgainstRepoNodeRowsToo(t *testing.T) {
	_, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	nodes.Rows = append(nodes.Rows, dtree.Node{ID: "repo-1", Kind: dtree.KindRepo, Order: 0})
	_, _, err := uc.Create(ctx, tree.CreateInput{ID: "a", Name: "a"})
	require.NoError(t, err)
	require.Equal(t, 1, nodeRowFor(t, nodes, "a").Order, "a landed after the repo already at the root")

	_, shifted, err := uc.Move(ctx, "a", tree.MoveInput{Order: index(0)})
	require.NoError(t, err)
	assert.Empty(t, shifted, "the repo is not a FOLDER row, so it never rides the folder broadcast list")

	assert.Equal(t, 0, nodeRowFor(t, nodes, "a").Order)
	assert.Equal(t, 1, nodeRowFor(t, nodes, "repo-1").Order,
		"the repo sibling shifted too, written back through the SAME Node surface")
}

func TestDelete_Home_PromotesChildrenAndErasesFolderAndNode(t *testing.T) {
	_, folders, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	parent, _, err := uc.Create(ctx, tree.CreateInput{ID: "parent", Name: "parent"})
	require.NoError(t, err)
	child, _, err := uc.Create(ctx, tree.CreateInput{ID: "child", ParentID: parent.ID, Name: "child"})
	require.NoError(t, err)

	_, err = uc.Delete(ctx, parent.ID)
	require.NoError(t, err)

	for _, f := range folders.Saved {
		assert.NotEqual(t, "parent", f.ID, "the deleted folder's identity row is gone")
	}
	assert.Equal(t, "", nodeRowFor(t, nodes, child.ID).ParentID, "the child is promoted to the root")
}

func TestPlaceChat_Home_DensifiesAgainstRepoAndFolderSiblings(t *testing.T) {
	chats, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	nodes.Rows = append(nodes.Rows, dtree.Node{ID: "repo-1", Kind: dtree.KindRepo, Order: 0})
	folder, _, err := uc.Create(ctx, tree.CreateInput{Name: "docs"})
	require.NoError(t, err)
	require.Equal(t, 1, nodeRowFor(t, nodes, folder.ID).Order)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID})

	_, _, err = uc.PlaceChat(ctx, homeWorkspaceID, "c1", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err)

	assert.Equal(t, dtree.KindChat, nodeRowFor(t, nodes, "c1").Kind)
	assert.Equal(t, 0, nodeRowFor(t, nodes, "c1").Order)
	assert.Equal(t, 1, nodeRowFor(t, nodes, "repo-1").Order, "shifted by the chat landing before it")
	assert.Equal(t, 2, nodeRowFor(t, nodes, folder.ID).Order, "shifted too")

	assert.Empty(t, chats.Placed, "a home chat's position write never touches the chat aggregate")
	assert.Empty(t, chats.Ordered, "a home chat's position write never touches the chat aggregate")
}

func TestPlaceChat_Home_FirstPlacementMintsANodeRow(t *testing.T) {
	chats, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID})
	folder, _, err := uc.Create(ctx, tree.CreateInput{Name: "docs"})
	require.NoError(t, err)

	placed, _, err := uc.PlaceChat(ctx, homeWorkspaceID, "c1", tree.PlaceInput{ParentID: name(folder.ID)})
	require.NoError(t, err)
	assert.Equal(t, folder.ID, placed.ParentID)

	n := nodeRowFor(t, nodes, "c1")
	assert.Equal(t, folder.ID, n.ParentID)
	assert.Equal(t, dtree.KindChat, n.Kind)
	assert.Empty(t, chats.Placed, "the write went to Node, never Chat.SetPlacement")
}

func TestPlaceChat_Home_LoneChatWithNoNodeRowStillMintsWhenTargetCoincidesWithZero(t *testing.T) {
	chats, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID})

	_, _, err := uc.PlaceChat(ctx, homeWorkspaceID, "c1", tree.PlaceInput{Order: index(0)})
	require.NoError(t, err, "a lone home chat with no Node row must mint one, not silently do nothing")

	n := nodeRowFor(t, nodes, "c1")
	assert.Equal(t, dtree.KindChat, n.Kind)
	assert.Equal(t, "", n.ParentID)
	assert.Equal(t, 0, n.Order)
}

func TestPlaceChat_Home_SecondMoveReadsLivePositionFromNodeNotChat(t *testing.T) {
	chats, _, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID})
	folderA, _, err := uc.Create(ctx, tree.CreateInput{ID: "fa", Name: "a"})
	require.NoError(t, err)
	folderB, _, err := uc.Create(ctx, tree.CreateInput{ID: "fb", Name: "b"})
	require.NoError(t, err)

	_, _, err = uc.PlaceChat(ctx, homeWorkspaceID, "c1", tree.PlaceInput{ParentID: name(folderA.ID)})
	require.NoError(t, err)
	_, _, err = uc.PlaceChat(ctx, homeWorkspaceID, "c1", tree.PlaceInput{ParentID: name(folderB.ID)})
	require.NoError(t, err)

	assert.Equal(t, folderB.ID, nodeRowFor(t, nodes, "c1").ParentID, "Node tracks the live position")
	assert.Equal(t, "", chatRow(t, chats, "c1").ParentID,
		"Chat's own field stays frozen at its creation-time value -- Task 11 deletes it, not this task")
	assert.Empty(t, chats.Placed)
	assert.Empty(t, chats.Ordered)
}

func TestListInRepo_Home_IsolatesFromRepoScoped(t *testing.T) {
	chats, _, _, uc := newHomeUsecase(t)
	ctx := context.Background()
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "f-repo", Type: domain.ChatTypeFolder, RepoID: repoID, Title: "repo-scoped"})
	created, _, err := uc.Create(ctx, tree.CreateInput{ID: "f-home", Name: "home"})
	require.NoError(t, err)

	rows, err := uc.ListInRepo(ctx, "")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, created.ID, rows[0].ID)
	assert.Equal(t, "home", rows[0].Title)
}

func TestListInHome_ListsOnlyTheFoldersOfThatHome(t *testing.T) {
	_, _, _, uc := newHomeUsecase(t)
	ctx := context.Background()
	mine, _, err := uc.Create(ctx, tree.CreateInput{ID: "f-mine", Name: "mine", HomeID: "home-a"})
	require.NoError(t, err)
	_, _, err = uc.Create(ctx, tree.CreateInput{ID: "f-theirs", Name: "theirs", HomeID: "home-b"})
	require.NoError(t, err)

	rows, err := uc.ListInHome(ctx, "home-a")

	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, mine.ID, rows[0].ID)
}

func TestListInHome_AdoptsALegacyFolderByTheHomeItsMemberChatRunsIn(t *testing.T) {
	chats, folders, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	folders.Saved = append(folders.Saved, dtree.Folder{ID: "f-legacy", Name: "old"})
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", WorkspaceID: "home-b"})
	nodes.Rows = append(nodes.Rows,
		dtree.Node{ID: "f-legacy", Kind: dtree.KindFolder},
		dtree.Node{ID: "c1", Kind: dtree.KindChat, ParentID: "f-legacy"})

	elsewhere, err := uc.ListInHome(ctx, "home-a")
	require.NoError(t, err)
	assert.Empty(t, elsewhere, "the lister is not the owner")
	owned, err := uc.ListInHome(ctx, "home-b")
	require.NoError(t, err)

	require.Len(t, owned, 1)
	assert.Equal(t, "f-legacy", owned[0].ID)
	scope, err := uc.FolderScope(ctx, "f-legacy")
	require.NoError(t, err)
	assert.Equal(t, "home-b", scope.HomeID, "the adoption is persisted")
}

func TestListInHome_AFolderNothingAttributesGoesToTheFirstHomeThatListsIt(t *testing.T) {
	_, folders, _, uc := newHomeUsecase(t)
	ctx := context.Background()
	folders.Saved = append(folders.Saved, dtree.Folder{ID: "f-legacy", Name: "old"})

	first, err := uc.ListInHome(ctx, "home-a")
	require.NoError(t, err)
	second, err := uc.ListInHome(ctx, "home-b")
	require.NoError(t, err)

	assert.Len(t, first, 1)
	assert.Empty(t, second)
}

func TestFolderScope_AnUnknownFolderIsNotFound(t *testing.T) {
	_, _, _, uc := newHomeUsecase(t)

	_, err := uc.FolderScope(context.Background(), "nope")

	require.ErrorIs(t, err, problem.ErrNotFound)
}

func TestListInHome_ALegacyFolderTakesTheHomeOfAFolderAroundIt(t *testing.T) {
	_, folders, nodes, uc := newHomeUsecase(t)
	ctx := context.Background()
	folders.Saved = append(folders.Saved,
		dtree.Folder{ID: "f-outer", Name: "outer"},
		dtree.Folder{ID: "f-inner", Name: "inner", HomeID: "home-c"},
		dtree.Folder{ID: "f-deep", Name: "deep"})
	nodes.Rows = append(nodes.Rows,
		dtree.Node{ID: "f-outer", Kind: dtree.KindFolder},
		dtree.Node{ID: "f-inner", Kind: dtree.KindFolder, ParentID: "f-outer"},
		dtree.Node{ID: "f-deep", Kind: dtree.KindFolder, ParentID: "f-inner"})

	rows, err := uc.ListInHome(ctx, "home-a")
	require.NoError(t, err)
	assert.Empty(t, rows, "neither legacy folder is home-a's")

	for _, id := range []string{"f-outer", "f-deep"} {
		scope, err := uc.FolderScope(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, "home-c", scope.HomeID, id)
	}
}
