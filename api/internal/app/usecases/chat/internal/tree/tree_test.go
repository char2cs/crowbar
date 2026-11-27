package tree_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

const (
	workspaceID = "ws-1"
	repoID      = "repo-1"
)

var errNoLog = errors.New("log unavailable")

func seedFolder(
	t *testing.T,
	uc tree.Usecase,
	id string,
	parentID string,
) {
	t.Helper()
	_, _, err := uc.Create(context.Background(), tree.CreateInput{
		ID: id, RepoID: repoID, ParentID: parentID, Name: id,
	})
	require.NoError(t, err)
}

func staleAt(
	chats *mocks.AgentChatPlacements,
	id string,
	parentID string,
	order int,
	createdAtSec int64,
) {
	if chats.Stale == nil {
		chats.Stale = map[string]domain.Chat{}
	}
	chats.Stale[id] = domain.Chat{
		ID:          id,
		Type:        domain.ChatTypeChat,
		WorkspaceID: workspaceID,
		ParentID:    parentID,
		Order:       order,
		CreatedAt:   time.Unix(createdAtSec, 0).UTC(),
	}
}

func TestDelete_PromotedChatsAreWrittenAsRealMoves(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	created, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)
	_, _, err = uc.PlaceChat(ctx, workspaceID, "c1",
		tree.PlaceInput{ParentID: name(created.ID)})
	require.NoError(t, err)
	nodes.Placed = nil

	_, err = uc.Delete(ctx, created.ID)
	require.NoError(t, err)

	assert.Equal(t, "", nodeRowFor(t, nodes, "c1").ParentID, "the chat came back up to the root")
	require.Len(t, nodes.Placed, 1)
	assert.Equal(t, "c1", nodes.Placed[0].ID)
	require.Empty(t, folderRows(t, uc), "and the folder itself is gone")
}

func newUsecase(
	t *testing.T,
) (*mocks.AgentChatPlacements, tree.Usecase) {
	t.Helper()
	chats, uc, _ := newUsecaseWithWork(t)
	return chats, uc
}

func seedChat(
	chats *mocks.AgentChatPlacements,
	id string,
	createdAtSec int64,
) {
	chats.Rows = append(chats.Rows, domain.Chat{
		ID:          id,
		Type:        domain.ChatTypeChat,
		WorkspaceID: workspaceID,
		CreatedAt:   time.Unix(createdAtSec, 0).UTC(),
	})
}

func seedThread(
	chats *mocks.AgentChatPlacements,
	id string,
	parentID string,
	createdAtSec int64,
) {
	chats.Rows = append(chats.Rows, domain.Chat{
		ID:          id,
		Type:        domain.ChatTypeChat,
		WorkspaceID: workspaceID,
		ParentID:    parentID,
		CreatedAt:   time.Unix(createdAtSec, 0).UTC(),
	})
}

func folderRow(
	t *testing.T,
	uc tree.Usecase,
	id string,
) domain.Chat {
	t.Helper()
	for _, row := range folderRows(t, uc) {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("folder %s not found", id)
	return domain.Chat{}
}

func chatRow(
	t *testing.T,
	chats *mocks.AgentChatPlacements,
	id string,
) domain.Chat {
	t.Helper()
	for _, c := range chats.Rows {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("chat %s not found", id)
	return domain.Chat{}
}

func folderRows(
	t *testing.T,
	uc tree.Usecase,
) []domain.Chat {
	t.Helper()
	rows, err := uc.ListInRepo(context.Background(), repoID)
	require.NoError(t, err)
	return rows
}

func name(v string) *string { return &v }

func index(v int) *int { return &v }

func TestCreate_AppendsAtTheEndOfTheSiblingSpace(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)

	created, shifted, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)
	assert.Equal(t, "spikes", created.Title)
	assert.Equal(t, domain.ChatTypeFolder, created.Type)
	assert.Equal(t, 2, created.Order, "a new folder lands after the rows already at that level")
	assert.Empty(t, shifted, "the folder is the only folder at this level")
	assert.NotEmpty(t, created.ID, "an id-less create mints one")

	assert.Equal(t, 0, chatRow(t, chats, "c1").Order)
	assert.Equal(t, 1, chatRow(t, chats, "c2").Order)
}

func TestCreate_HonoursACallerSuppliedID(t *testing.T) {
	_, uc := newUsecase(t)

	created, _, err := uc.Create(context.Background(), tree.CreateInput{
		ID: "f-fixed", RepoID: repoID, Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, "f-fixed", created.ID)
}

func TestCreate_TrimsAndRefusesABlankName(t *testing.T) {
	_, uc := newUsecase(t)
	ctx := context.Background()

	created, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "  spikes  "})
	require.NoError(t, err)
	assert.Equal(t, "spikes", created.Title)

	_, _, err = uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "   "})
	assert.ErrorIs(t, err, tree.ErrNameRequired)
}

func TestCreate_NestsInsideAChat(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)

	created, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "c1", Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, "c1", created.ParentID)
	assert.Equal(t, "c1", folderRow(t, uc, created.ID).ParentID)
}

func TestCreate_RefusesAParentThatDoesNotExist(t *testing.T) {
	_, uc := newUsecase(t)

	_, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "nowhere", Name: "spikes",
	})
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestCreate_RefusesAChatParentFromAnotherRepo(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", Type: domain.ChatTypeChat, WorkspaceID: "ws-2"})

	_, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "c-other", Name: "spikes",
	})
	assert.ErrorIs(t, err, tree.ErrCrossContext)
}

func TestCreate_AcceptsAChatParentFromTheSameRepo(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", Type: domain.ChatTypeChat, WorkspaceID: workspaceID})

	created, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "c-other", Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, "c-other", created.ParentID)
}

func TestCreate_AcceptsAChatTheGlobalListDidNotCarry(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.MissingID = "c1"

	created, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "c1", Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, "c1", created.ParentID)
}

func TestCreate_SurfacesASnapshotFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.ListErr = errors.New("boom")

	_, _, err := uc.Create(context.Background(), tree.CreateInput{RepoID: repoID, Name: "spikes"})
	assert.ErrorContains(t, err, "boom")
}

func TestCreate_SurfacesAParentLookupFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.GetErr = errors.New("key read down")

	_, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "nowhere", Name: "spikes",
	})
	assert.ErrorContains(t, err, "key read down")
}

func TestCreate_SurfacesACreateFailure(t *testing.T) {
	_, folders, _, uc, _ := newUsecaseWithStores(t)
	folders.SaveErr = errors.New("aggregate wedged")

	_, _, err := uc.Create(context.Background(), tree.CreateInput{RepoID: repoID, Name: "spikes"})
	assert.ErrorContains(t, err, "aggregate wedged")
}

func TestCreate_SurfacesATitleFailure(t *testing.T) {
	_, folders, nodes, uc, _ := newUsecaseWithStores(t)
	folders.SaveErr = errors.New("title rejected")

	_, _, err := uc.Create(context.Background(), tree.CreateInput{ID: "f-new", RepoID: repoID, Name: "spikes"})
	assert.ErrorContains(t, err, "title rejected")
	assert.Empty(t, folders.Saved, "the half-created folder must be discarded")
	assert.Empty(t, nodes.Rows, "and its Node row, though Save never having run means Forget is a no-op here")
}

func TestCreate_SurfacesAChatRenumberFailure(t *testing.T) {
	chats, folders, _, uc, _ := newUsecaseWithStores(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	chats.OrderErr = errors.New("aggregate down")

	_, _, err := uc.Create(context.Background(), tree.CreateInput{ID: "f-new", RepoID: repoID, Name: "spikes"})
	assert.ErrorContains(t, err, "aggregate down")
	assert.Empty(t, folders.Saved,
		"a sibling renumber failure after the mint must still discard the half-created folder")
}

func TestCreate_DiscardsTheFolderWhenItsOwnPlacementWriteFails(t *testing.T) {
	_, folders, nodes, uc, _ := newUsecaseWithStores(t)
	nodes.CreateErr = errors.New("wedged")

	_, _, err := uc.Create(context.Background(), tree.CreateInput{ID: "f-new", RepoID: repoID, Name: "spikes"})
	assert.ErrorContains(t, err, "wedged")
	assert.Empty(t, folders.Saved, "the half-created folder's identity row must be discarded too")
	assert.Empty(t, nodes.Rows, "and its Node row (though Create never succeeding means Forget is a no-op)")
}

func TestListInRepo_ReturnsOnlyFolderTypedRows(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(t, uc, "f1", "")
	seedChat(chats, "c1", 1)

	rows, err := uc.ListInRepo(context.Background(), repoID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "f1", rows[0].ID)
}

func TestListInRepo_IsolatesByRepo(t *testing.T) {
	_, folders, _, uc, _ := newUsecaseWithStores(t)
	seedFolder(t, uc, "f-this-repo", "")
	require.NoError(t, folders.Save(context.Background(),
		dtree.Folder{ID: "f-other-repo", Name: "theirs", RepoID: "repo-2"}))

	rows, err := uc.ListInRepo(context.Background(), repoID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "f-this-repo", rows[0].ID)
}

func TestListInRepo_SurfacesAStoreFailure(t *testing.T) {
	_, folders, _, uc, _ := newUsecaseWithStores(t)
	folders.FindErr = errors.New("boom")

	_, err := uc.ListInRepo(context.Background(), repoID)
	assert.ErrorContains(t, err, "boom")
}

func TestRename_TrimsAndRefusesABlankName(t *testing.T) {
	_, uc := newUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "old"})
	require.NoError(t, err)

	renamed, err := uc.Rename(ctx, created.ID, "  new  ")
	require.NoError(t, err)
	assert.Equal(t, "new", renamed.Title)
	assert.Equal(t, "new", folderRow(t, uc, created.ID).Title)

	_, err = uc.Rename(ctx, created.ID, " ")
	assert.ErrorIs(t, err, tree.ErrNameRequired)
}

func TestRename_RefusesAChatTypedID(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)

	_, err := uc.Rename(context.Background(), "c1", "new")
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestRename_RefusesAnUnknownID(t *testing.T) {
	_, uc := newUsecase(t)

	_, err := uc.Rename(context.Background(), "nowhere", "new")
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestRename_SurfacesAReadFailure(t *testing.T) {
	_, folders, _, uc, _ := newUsecaseWithStores(t)
	folders.FindByKeyErr = errors.New("boom")

	_, err := uc.Rename(context.Background(), "f1", "new")
	assert.ErrorContains(t, err, "boom")
}

func TestRename_SurfacesASaveFailure(t *testing.T) {
	_, folders, _, uc, _ := newUsecaseWithStores(t)
	seedFolder(t, uc, "f1", "")
	folders.SaveErr = errors.New("disk full")

	_, err := uc.Rename(context.Background(), "f1", "new")
	assert.ErrorContains(t, err, "disk full")
}

func TestMove_DensifiesBothLevels(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	container, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "container"})
	require.NoError(t, err)
	moved, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)

	placed, shifted, err := uc.Move(ctx, moved.ID, tree.MoveInput{
		ParentID: name(container.ID),
	})
	require.NoError(t, err)
	assert.Equal(t, container.ID, placed.ParentID)
	assert.Equal(t, 0, placed.Order, "the destination level was empty")
	assert.Empty(t, shifted, "no other FOLDER moved")
	assert.Equal(t, 0, chatRow(t, chats, "c1").Order)
	assert.Equal(t, 1, chatRow(t, chats, "c2").Order, "the level it left is renumbered too")
}

func TestMove_WithNothingRequestedKeepsThePlacement(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	created, _, err := uc.Create(ctx, tree.CreateInput{
		RepoID: repoID, ParentID: "c1", Name: "spikes",
	})
	require.NoError(t, err)

	placed, _, err := uc.Move(ctx, created.ID, tree.MoveInput{})
	require.NoError(t, err)
	assert.Equal(t, "c1", placed.ParentID)
	assert.Equal(t, 0, placed.Order)
}

func TestMove_ReordersWithinALevelAndReportsTheCollateral(t *testing.T) {
	_, uc := newUsecase(t)
	ctx := context.Background()
	first, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "a"})
	require.NoError(t, err)
	second, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "b"})
	require.NoError(t, err)

	placed, shifted, err := uc.Move(ctx, second.ID, tree.MoveInput{Order: index(0)})
	require.NoError(t, err)
	assert.Equal(t, 0, placed.Order)
	require.Len(t, shifted, 1)
	assert.Equal(t, first.ID, shifted[0].ID)
	assert.Equal(t, 1, shifted[0].Order)
}

func TestMove_RefusesAFolderOntoItself(t *testing.T) {
	_, uc := newUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)

	_, _, err = uc.Move(ctx, created.ID, tree.MoveInput{ParentID: name(created.ID)})
	assert.ErrorIs(t, err, tree.ErrCycle)
}

func TestMove_RefusesAMoveIntoItsOwnSubtree(t *testing.T) {
	_, uc := newUsecase(t)
	ctx := context.Background()
	outer, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "outer"})
	require.NoError(t, err)
	inner, _, err := uc.Create(ctx, tree.CreateInput{
		RepoID: repoID, ParentID: outer.ID, Name: "inner",
	})
	require.NoError(t, err)

	_, _, err = uc.Move(ctx, outer.ID, tree.MoveInput{ParentID: name(inner.ID)})
	assert.ErrorIs(t, err, tree.ErrCycle)
}

func TestMove_RefusesAnUnknownID(t *testing.T) {
	_, uc := newUsecase(t)

	_, _, err := uc.Move(context.Background(), "nowhere", tree.MoveInput{})
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestMove_SurfacesASnapshotFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)
	chats.ListErr = errors.New("chats down")

	_, _, err = uc.Move(ctx, created.ID, tree.MoveInput{})
	assert.ErrorContains(t, err, "chats down")
}

func TestMove_RefusesAParentThatDoesNotExist(t *testing.T) {
	_, uc := newUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)

	_, _, err = uc.Move(ctx, created.ID, tree.MoveInput{ParentID: name("nowhere")})
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestMove_SurfacesAnOrderFailure(t *testing.T) {
	_, _, nodes, uc, _ := newUsecaseWithStores(t)
	ctx := context.Background()
	_, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "a"})
	require.NoError(t, err)
	second, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "b"})
	require.NoError(t, err)
	nodes.OrderErr = errors.New("disk full")

	_, _, err = uc.Move(ctx, second.ID, tree.MoveInput{Order: index(0)})
	assert.ErrorContains(t, err, "disk full")
}

func TestMove_SurfacesAPlacementFailure(t *testing.T) {
	_, _, nodes, uc, _ := newUsecaseWithStores(t)
	ctx := context.Background()
	outer, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "outer"})
	require.NoError(t, err)
	moved, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "moved"})
	require.NoError(t, err)
	nodes.PlaceErr = errors.New("wedged")

	_, _, err = uc.Move(ctx, moved.ID, tree.MoveInput{ParentID: name(outer.ID)})
	assert.ErrorContains(t, err, "wedged")
}

func TestDelete_PromotesChildrenToTheFoldersOwnParent(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	outer, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "outer"})
	require.NoError(t, err)
	inner, _, err := uc.Create(ctx, tree.CreateInput{
		RepoID: repoID, ParentID: outer.ID, Name: "inner",
	})
	require.NoError(t, err)
	_, _, err = uc.PlaceChat(ctx, workspaceID, "c1", tree.PlaceInput{ParentID: name(outer.ID)})
	require.NoError(t, err)

	written, err := uc.Delete(ctx, outer.ID)
	require.NoError(t, err)

	assert.Equal(t, "", folderRow(t, uc, inner.ID).ParentID, "the child folder rises to the root")
	assert.Equal(t, "", chatRow(t, chats, "c1").ParentID, "the chat survives its folder")
	ids := make([]string, 0, len(written))
	for _, row := range written {
		ids = append(ids, row.ID)
	}
	assert.Equal(t, []string{inner.ID}, ids, "the promoted folder rows come back for broadcast")
}

func TestDelete_RefusesWorkingChild(t *testing.T) {
	chats, _, nodes, uc, work := newUsecaseWithStores(t)
	ctx := context.Background()
	outer, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "outer"})
	require.NoError(t, err)
	seedChat(chats, "c1", 1)
	_, _, err = uc.PlaceChat(ctx, workspaceID, "c1", tree.PlaceInput{ParentID: name(outer.ID)})
	require.NoError(t, err)
	work.Set("c1", true)

	_, err = uc.Delete(ctx, outer.ID)
	assert.ErrorIs(t, err, tree.ErrSubtreeWorking)
	assert.Equal(t, outer.ID, nodeRowFor(t, nodes, "c1").ParentID, "a refused delete promotes nothing")
}

func TestDelete_RefusesAnUnknownID(t *testing.T) {
	_, uc := newUsecase(t)

	_, err := uc.Delete(context.Background(), "nowhere")
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestDelete_SurfacesASnapshotFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)
	chats.ListErr = errors.New("chats down")

	_, err = uc.Delete(ctx, created.ID)
	assert.ErrorContains(t, err, "chats down")
}

func TestDelete_SurfacesARemovalFailure(t *testing.T) {
	_, folders, _, uc, _ := newUsecaseWithStores(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)
	folders.DeleteErr = errors.New("locked")

	_, err = uc.Delete(ctx, created.ID)
	assert.ErrorContains(t, err, "locked")
}

func TestDeleteChat_TakesTheWholeSubtreeDeepestFirst(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "root", 1)
	seedThread(chats, "child", "root", 2)
	seedThread(chats, "grandchild", "child", 3)
	seedChat(chats, "bystander", 4)

	removed, err := uc.DeleteChat(ctx, "root", domain.KeepWorkAtRisk)
	require.NoError(t, err)

	assert.Equal(t, []string{"grandchild", "child", "root"}, chats.Purged)
	assert.Equal(t, []string{"grandchild", "child", "root"}, removed.Chats)
	assert.Empty(t, removed.Folders)
	require.Len(t, chats.Rows, 1)
	assert.Equal(t, "bystander", chats.Rows[0].ID)
}

func TestDeleteChat_TakesTheFoldersCaughtInTheSubtree(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "root", 1)
	inside, _, err := uc.Create(ctx, tree.CreateInput{
		RepoID: repoID, ParentID: "root", Name: "spikes",
	})
	require.NoError(t, err)
	seedThread(chats, "filed", inside.ID, 2)

	removed, err := uc.DeleteChat(ctx, "root", domain.KeepWorkAtRisk)
	require.NoError(t, err)

	assert.Equal(t, []string{"filed", "root"}, removed.Chats)
	assert.Equal(t, []string{inside.ID}, removed.Folders)
	assert.Empty(t, folderRows(t, uc), "the folder went with the chat that held it")
}

func TestDeleteChat_DensifiesTheLevelItLeft(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	folder, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)
	require.Equal(t, 1, folder.Order)

	removed, err := uc.DeleteChat(ctx, "c1", domain.KeepWorkAtRisk)
	require.NoError(t, err)

	require.Len(t, removed.Shifted, 1)
	assert.Equal(t, folder.ID, removed.Shifted[0].ID)
	assert.Equal(t, 0, removed.Shifted[0].Order)
}

func TestDeleteChat_RefusesWorkingSubtree(t *testing.T) {
	chats, uc, work := newUsecaseWithWork(t)
	ctx := context.Background()
	seedChat(chats, "root", 1)
	seedThread(chats, "child", "root", 2)
	work.Set("child", true)

	_, err := uc.DeleteChat(ctx, "root", domain.KeepWorkAtRisk)
	assert.ErrorIs(t, err, tree.ErrSubtreeWorking)
	assert.Empty(t, chats.Purged, "nothing may be torn down once any row in the subtree refuses")
}

func TestDeleteChat_RefusesTheNamedChatItselfWhenWorking(t *testing.T) {
	chats, uc, work := newUsecaseWithWork(t)
	ctx := context.Background()
	seedChat(chats, "solo", 1)
	work.Set("solo", true)

	_, err := uc.DeleteChat(ctx, "solo", domain.KeepWorkAtRisk)
	assert.ErrorIs(t, err, tree.ErrSubtreeWorking)
	assert.Empty(t, chats.Purged, "the working leaf itself must never be purged")
}

func TestDeleteChat_IdleSubtreeCascadesDespiteAWorkingBystander(t *testing.T) {
	chats, uc, work := newUsecaseWithWork(t)
	ctx := context.Background()
	seedChat(chats, "root", 1)
	seedThread(chats, "child", "root", 2)
	seedChat(chats, "bystander", 3)
	work.Set("bystander", true)

	removed, err := uc.DeleteChat(ctx, "root", domain.KeepWorkAtRisk)
	require.NoError(t, err)
	assert.Equal(t, []string{"child", "root"}, removed.Chats)
}

func TestDeleteChat_RefusesAnUnknownChat(t *testing.T) {
	_, uc := newUsecase(t)

	_, err := uc.DeleteChat(context.Background(), "nowhere", domain.KeepWorkAtRisk)
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestDeleteChat_SurfacesASnapshotFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.ListErr = errors.New("boom")

	_, err := uc.DeleteChat(context.Background(), "c1", domain.KeepWorkAtRisk)
	assert.ErrorContains(t, err, "boom")
}

func TestDeleteChat_SurfacesAPurgeFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.PurgeErr = errors.New("cli wedged")

	_, err := uc.DeleteChat(context.Background(), "c1", domain.KeepWorkAtRisk)
	assert.ErrorContains(t, err, "cli wedged")
}

func TestDeleteChat_ToleratesAnAlreadyGoneDescendantAndStillDeletesTheRest(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "root", 1)
	seedThread(chats, "ghost", "root", 2)
	seedThread(chats, "grandchild", "ghost", 3)
	chats.PurgeNotFoundID = "ghost"

	removed, err := uc.DeleteChat(ctx, "root", domain.KeepWorkAtRisk)

	require.NoError(t, err)
	assert.Equal(t, []string{"grandchild", "ghost", "root"}, chats.Purged,
		"the not-found descendant does not stop the cascade around it")
	assert.Equal(t, []string{"grandchild", "ghost", "root"}, removed.Chats)
	assert.Empty(t, chats.Rows, "every row in the subtree is gone, ghost included")
}

func TestDeleteChat_SurfacesAFolderRemovalFailure(t *testing.T) {
	chats, folders, _, uc, _ := newUsecaseWithStores(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	_, _, err := uc.Create(ctx, tree.CreateInput{
		RepoID: repoID, ParentID: "c1", Name: "spikes",
	})
	require.NoError(t, err)
	folders.DeleteErr = errors.New("locked")

	_, err = uc.DeleteChat(ctx, "c1", domain.KeepWorkAtRisk)
	assert.ErrorContains(t, err, "locked")
}

func TestDeleteChat_SurfacesADensifyWriteFailure(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	_, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)
	nodes.OrderErr = errors.New("disk full")

	_, err = uc.DeleteChat(ctx, "c1", domain.KeepWorkAtRisk)
	assert.ErrorContains(t, err, "disk full")
}

func TestRegression_DeleteChat_ToleratesASiblingPurgedByAConcurrentDelete(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "root", 1)
	seedThread(chats, "a", "root", 2)
	seedThread(chats, "b", "root", 3)
	for i := range chats.Rows {
		if chats.Rows[i].ID == "b" {
			chats.Rows[i].Order = 1
		}
	}
	chats.OrderNotFoundIDs = map[string]bool{"b": true}

	removed, err := uc.DeleteChat(ctx, "a", domain.KeepWorkAtRisk)

	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, removed.Chats)
}

func TestDeleteChat_ProceedsWhenTheListLagsTheAggregate(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.MissingID = "c1"

	removed, err := uc.DeleteChat(context.Background(), "c1", domain.KeepWorkAtRisk)
	require.NoError(t, err)
	assert.Equal(t, []string{"c1"}, removed.Chats)
	assert.Equal(t, []string{"c1"}, chats.Purged)
}
