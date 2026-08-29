package tree_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

const workspaceID = "ws-1"
const repoID = "repo-1"

// errNoLog stands in for the event log being unreachable.
var errNoLog = errors.New("log unavailable")

// seedFolder appends a folder row hanging off parentID. Folders carry no
// workspace: they are a domain.Chat row like any other, distinguished only by
// Type.
func seedFolder(
	chats *mocks.AgentChatPlacements,
	id string,
	parentID string,
) {
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: id, Type: domain.ChatTypeFolder, ParentID: parentID, Title: id,
	})
}

// staleAt holds the PROJECTION of a chat at a placement it no longer has, which
// is the daemon's ordinary state for as long as the read model trails the log
// after a write — a window every second call of a multi-row drag lands inside.
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

// Deleting a folder promotes what was inside it, so those rows really did change
// level and their parents really must be written — the fix must not turn every
// chat write into a renumber.
func TestDelete_PromotedChatsAreWrittenAsRealMoves(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	created, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)
	_, _, err = uc.PlaceChat(ctx, workspaceID, "c1",
		tree.PlaceInput{ParentID: name(created.ID)})
	require.NoError(t, err)
	chats.Placed = nil

	_, err = uc.Delete(ctx, created.ID)
	require.NoError(t, err)

	assert.Equal(t, "", chatRow(t, chats, "c1").ParentID, "the chat came back up to the root")
	require.Len(t, chats.Placed, 1)
	assert.Equal(t, "c1", chats.Placed[0].ChatID)
	require.Empty(t, folderRows(t, chats), "and the folder itself is gone")
}

func newUsecase(
	t *testing.T,
) (*mocks.AgentChatPlacements, tree.Usecase) {
	t.Helper()
	chats, uc, _ := newUsecaseWithWork(t)
	return chats, uc
}

// seedChat appends a chat at the panel root, created at the given second so the
// creation-order tiebreak is deterministic.
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

// seedThread appends a chat threaded off parentID.
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
	chats *mocks.AgentChatPlacements,
	id string,
) domain.Chat {
	t.Helper()
	for _, row := range chats.Rows {
		if row.ID == id && row.Type == domain.ChatTypeFolder {
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

// folderRows is every FOLDER-typed row the store still holds.
func folderRows(
	t *testing.T,
	chats *mocks.AgentChatPlacements,
) []domain.Chat {
	t.Helper()
	rows := make([]domain.Chat, 0)
	for _, row := range chats.Rows {
		if row.Type == domain.ChatTypeFolder {
			rows = append(rows, row)
		}
	}
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

	// The densify runs over the WHOLE sibling space, so the two chats that were
	// both sitting on the migration default of 0 come out distinct.
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

// A folder INSIDE a chat is the case that makes this tree different from the
// sidebar's: it holds no turns, so it can order a chat's threads without ever
// being mistaken for one.
func TestCreate_NestsInsideAChat(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)

	created, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "c1", Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, "c1", created.ParentID)
	assert.Equal(t, "c1", folderRow(t, chats, created.ID).ParentID)
}

func TestCreate_RefusesAParentThatDoesNotExist(t *testing.T) {
	_, uc := newUsecase(t)

	_, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "nowhere", Name: "spikes",
	})
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

// A folder carries no workspace of its own (§3.1), so a chat parent is accepted
// regardless of which workspace it belongs to. Enforcing a repo boundary here
// is stage 3's walk, not this task's storage retype — this pins the current,
// deliberately permissive behaviour so a future tightening is a conscious
// assertion change, not a silent regression nobody noticed.
func TestCreate_AcceptsAChatParentFromAnyWorkspace(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", Type: domain.ChatTypeChat, WorkspaceID: "ws-2"})

	created, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "c-other", Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, "c-other", created.ParentID)
}

// The keyed chat read heals the chat read model for the one id it is asked
// about; the global list only heals a model that is entirely empty. So a
// parent the list did not carry can still be a legitimate container, and
// refusing it would reject a drop onto a chat the user can see.
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
	chats, uc := newUsecase(t)
	chats.CreateErr = errors.New("aggregate wedged")

	_, _, err := uc.Create(context.Background(), tree.CreateInput{RepoID: repoID, Name: "spikes"})
	assert.ErrorContains(t, err, "aggregate wedged")
}

func TestCreate_SurfacesATitleFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.TitleErr = errors.New("title rejected")

	_, _, err := uc.Create(context.Background(), tree.CreateInput{ID: "f-new", RepoID: repoID, Name: "spikes"})
	assert.ErrorContains(t, err, "title rejected")
	assert.Equal(t, []string{"f-new"}, chats.Forgotten, "the unnamed half-created folder must be discarded")
}

// A folder create renumbers the chats already at that level and moves none of
// them, so the chat write it can fail on is the renumber.
// A create that mints and names the folder successfully but fails during the
// densify that follows must not leave that row behind: the user was told the
// create failed, and CreateChat's own discard (chats.go) sets the precedent
// this mirrors — the whole post-mint sequence is covered, not just naming.
func TestCreate_SurfacesAChatRenumberFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	chats.OrderErr = errors.New("aggregate down")

	_, _, err := uc.Create(context.Background(), tree.CreateInput{ID: "f-new", RepoID: repoID, Name: "spikes"})
	assert.ErrorContains(t, err, "aggregate down")
	assert.Equal(t, []string{"f-new"}, chats.Forgotten,
		"a sibling renumber failure after the mint must still discard the half-created folder")
}

// The failure covered above is a SIBLING's renumber; this is the new folder's
// OWN placement write failing instead — discard must cover both call shapes
// persist can take.
func TestCreate_DiscardsTheFolderWhenItsOwnPlacementWriteFails(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.SetErr = errors.New("wedged")

	_, _, err := uc.Create(context.Background(), tree.CreateInput{ID: "f-new", RepoID: repoID, Name: "spikes"})
	assert.ErrorContains(t, err, "wedged")
	assert.Equal(t, []string{"f-new"}, chats.Forgotten)
}

// ListInRepo filters to folder-typed rows. It does not yet enforce a repo
// boundary — see Chats.ListChats's doc comment — so this proves the row-kind
// filter, not repo isolation.
func TestListInRepo_ReturnsOnlyFolderTypedRows(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(chats, "f1", "")
	seedChat(chats, "c1", 1)

	rows, err := uc.ListInRepo(context.Background(), repoID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "f1", rows[0].ID)
}

func TestListInRepo_SurfacesAStoreFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.ListErr = errors.New("boom")

	_, err := uc.ListInRepo(context.Background(), repoID)
	assert.ErrorContains(t, err, "boom")
}

func TestRename_TrimsAndRefusesABlankName(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "old"})
	require.NoError(t, err)

	renamed, err := uc.Rename(ctx, created.ID, "  new  ")
	require.NoError(t, err)
	assert.Equal(t, "new", renamed.Title)
	assert.Equal(t, "new", folderRow(t, chats, created.ID).Title)

	_, err = uc.Rename(ctx, created.ID, " ")
	assert.ErrorIs(t, err, tree.ErrNameRequired)
}

// Renaming an id that names a CHAT, not a folder, is refused as not-found: from
// this API's own vocabulary that id does not name a folder, and answering
// otherwise would let a rename reach a conversation through the wrong door.
func TestRename_RefusesAChatTypedID(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)

	_, err := uc.Rename(context.Background(), "c1", "new")
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestRename_RefusesAnUnknownID(t *testing.T) {
	_, uc := newUsecase(t)

	_, err := uc.Rename(context.Background(), "nowhere", "new")
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestRename_SurfacesAReadFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.LoadErr = errors.New("boom")

	_, err := uc.Rename(context.Background(), "f1", "new")
	assert.ErrorContains(t, err, "boom")
}

func TestRename_SurfacesASaveFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(chats, "f1", "")
	chats.TitleErr = errors.New("disk full")

	_, err := uc.Rename(context.Background(), "f1", "new")
	assert.ErrorContains(t, err, "disk full")
}

// Both levels are left dense: the one the row joined, and the one it left.
func TestMove_DensifiesBothLevels(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	moved, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)

	placed, shifted, err := uc.Move(ctx, moved.ID, tree.MoveInput{
		ParentID: name("c1"),
	})
	require.NoError(t, err)
	assert.Equal(t, "c1", placed.ParentID)
	assert.Equal(t, 0, placed.Order, "the destination level was empty")
	assert.Empty(t, shifted, "no other FOLDER moved")
	assert.Equal(t, 0, chatRow(t, chats, "c1").Order)
	assert.Equal(t, 1, chatRow(t, chats, "c2").Order, "the level it left is renumbered too")
}

// A move with no order and no parent change is a no-op placement whose only job
// is to report the row's real state — the shape a rename-only PATCH produces.
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

// An explicit index reorders WITHIN one level and reports every sibling the
// renumber moved, so no client is left holding stale orders.
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

// A move into a folder's own subtree would leave a set of rows unreachable from
// the panel root: they exist, nothing renders them, and nothing can drag them
// back out.
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
	assert.ErrorIs(t, err, apperr.ErrNotFound)
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
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

// A same-level reorder writes only indices, never a parent, so the failure it
// can surface is the order write.
func TestMove_SurfacesAnOrderFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	_, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "a"})
	require.NoError(t, err)
	second, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "b"})
	require.NoError(t, err)
	chats.OrderErr = errors.New("disk full")

	_, _, err = uc.Move(ctx, second.ID, tree.MoveInput{Order: index(0)})
	assert.ErrorContains(t, err, "disk full")
}

// A move that crosses into a different container writes the subject's
// placement whole.
func TestMove_SurfacesAPlacementFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	outer, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "outer"})
	require.NoError(t, err)
	moved, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "moved"})
	require.NoError(t, err)
	chats.SetErr = errors.New("wedged")

	_, _, err = uc.Move(ctx, moved.ID, tree.MoveInput{ParentID: name(outer.ID)})
	assert.ErrorContains(t, err, "wedged")
}

// A folder holds no conversation, so what it held outlives it. This is the
// opposite of deleting a CHAT — see the cascade tests below.
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

	assert.Equal(t, "", folderRow(t, chats, inner.ID).ParentID, "the child folder rises to the root")
	assert.Equal(t, "", chatRow(t, chats, "c1").ParentID, "the chat survives its folder")
	ids := make([]string, 0, len(written))
	for _, row := range written {
		ids = append(ids, row.ID)
	}
	assert.Equal(t, []string{inner.ID}, ids, "the promoted folder rows come back for broadcast")
}

func TestDelete_RefusesAnUnknownID(t *testing.T) {
	_, uc := newUsecase(t)

	_, err := uc.Delete(context.Background(), "nowhere")
	assert.ErrorIs(t, err, apperr.ErrNotFound)
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
	chats, uc := newUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)
	chats.ForgetErr = errors.New("locked")

	_, err = uc.Delete(ctx, created.ID)
	assert.ErrorContains(t, err, "locked")
}

// The cascade, and the reason it exists: a thread reads its parent's turns, so
// leaving it behind would strand a conversation whose whole premise is gone.
// Deepest first, so no intermediate state ever has a chat pointing at a parent
// that has already been erased.
func TestDeleteChat_TakesTheWholeSubtreeDeepestFirst(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "root", 1)
	seedThread(chats, "child", "root", 2)
	seedThread(chats, "grandchild", "child", 3)
	seedChat(chats, "bystander", 4)

	removed, err := uc.DeleteChat(ctx, "root")
	require.NoError(t, err)

	assert.Equal(t, []string{"grandchild", "child", "root"}, chats.Purged)
	assert.Equal(t, []string{"grandchild", "child", "root"}, removed.Chats)
	assert.Empty(t, removed.Folders)
	require.Len(t, chats.Rows, 1)
	assert.Equal(t, "bystander", chats.Rows[0].ID)
}

// A folder inside a deleted chat's subtree ordered that chat's threads. With the
// chat gone it has nothing left to order, so it goes too.
func TestDeleteChat_TakesTheFoldersCaughtInTheSubtree(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "root", 1)
	inside, _, err := uc.Create(ctx, tree.CreateInput{
		RepoID: repoID, ParentID: "root", Name: "spikes",
	})
	require.NoError(t, err)
	seedThread(chats, "filed", inside.ID, 2)

	removed, err := uc.DeleteChat(ctx, "root")
	require.NoError(t, err)

	assert.Equal(t, []string{"filed", "root"}, removed.Chats)
	assert.Equal(t, []string{inside.ID}, removed.Folders)
	assert.Empty(t, folderRows(t, chats), "the folder went with the chat that held it")
}

// The level the deleted chat left is renumbered, and the folders that moved come
// back so no client holds a stale order.
func TestDeleteChat_DensifiesTheLevelItLeft(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	folder, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)
	require.Equal(t, 1, folder.Order)

	removed, err := uc.DeleteChat(ctx, "c1")
	require.NoError(t, err)

	require.Len(t, removed.Shifted, 1)
	assert.Equal(t, folder.ID, removed.Shifted[0].ID)
	assert.Equal(t, 0, removed.Shifted[0].Order)
}

func TestDeleteChat_RefusesAnUnknownChat(t *testing.T) {
	_, uc := newUsecase(t)

	_, err := uc.DeleteChat(context.Background(), "nowhere")
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestDeleteChat_SurfacesASnapshotFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.ListErr = errors.New("boom")

	_, err := uc.DeleteChat(context.Background(), "c1")
	assert.ErrorContains(t, err, "boom")
}

func TestDeleteChat_SurfacesAPurgeFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.PurgeErr = errors.New("cli wedged")

	_, err := uc.DeleteChat(context.Background(), "c1")
	assert.ErrorContains(t, err, "cli wedged")
}

func TestDeleteChat_SurfacesAFolderRemovalFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	_, _, err := uc.Create(ctx, tree.CreateInput{
		RepoID: repoID, ParentID: "c1", Name: "spikes",
	})
	require.NoError(t, err)
	chats.ForgetErr = errors.New("locked")

	_, err = uc.DeleteChat(ctx, "c1")
	assert.ErrorContains(t, err, "locked")
}

func TestDeleteChat_SurfacesADensifyWriteFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	_, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)
	chats.OrderErr = errors.New("disk full")

	_, err = uc.DeleteChat(ctx, "c1")
	assert.ErrorContains(t, err, "disk full")
}

// A chat the snapshot's list did not carry is still deleted: the keyed read is
// the authority, and the densify simply counts the rows that were there.
func TestDeleteChat_ProceedsWhenTheListLagsTheAggregate(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.MissingID = "c1"

	removed, err := uc.DeleteChat(context.Background(), "c1")
	require.NoError(t, err)
	assert.Equal(t, []string{"c1"}, removed.Chats)
	assert.Equal(t, []string{"c1"}, chats.Purged)
}
