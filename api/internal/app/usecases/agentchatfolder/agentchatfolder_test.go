package agentchatfolder_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agentchatfolder"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

const workspaceID = "ws-1"

func newUsecase(
	t *testing.T,
) (*mocks.AgentChatFolderStore, *mocks.AgentChatPlacements, agentchatfolder.Usecase) {
	t.Helper()
	folders := mocks.NewAgentChatFolderStore()
	chats := mocks.NewAgentChatPlacements()
	return folders, chats, agentchatfolder.New(folders, chats, chats)
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
		WorkspaceID: workspaceID,
		ParentID:    parentID,
		CreatedAt:   time.Unix(createdAtSec, 0).UTC(),
	})
}

func folderRow(
	t *testing.T,
	folders *mocks.AgentChatFolderStore,
	id string,
) domain.ChatFolder {
	t.Helper()
	row, err := folders.FindByKey(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, row, "folder %s must exist", id)
	return *row
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

// folderRows is every folder the store still holds for this workspace.
func folderRows(
	t *testing.T,
	folders *mocks.AgentChatFolderStore,
) []domain.ChatFolder {
	t.Helper()
	rows, err := folders.FindWhere(context.Background(), domain.ChatFolder{WorkspaceID: workspaceID})
	require.NoError(t, err)
	return rows
}

// errNoLog stands in for the event log being unreachable.
var errNoLog = errors.New("log unavailable")

func name(v string) *string { return &v }
func index(v int) *int      { return &v }

func TestCreate_AppendsAtTheEndOfTheSiblingSpace(t *testing.T) {
	_, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)

	created, shifted, err := uc.Create(ctx, agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, "spikes", created.Name)
	assert.Equal(t, 2, created.Order, "a new folder lands after the rows already at that level")
	assert.Empty(t, shifted, "the folder is the only folder at this level")
	assert.NotEmpty(t, created.ID, "an id-less create mints one")

	// The densify runs over the WHOLE sibling space, so the two chats that were
	// both sitting on the migration default of 0 come out distinct.
	assert.Equal(t, 0, chatRow(t, chats, "c1").Order)
	assert.Equal(t, 1, chatRow(t, chats, "c2").Order)
}

func TestCreate_HonoursACallerSuppliedID(t *testing.T) {
	_, _, uc := newUsecase(t)

	created, _, err := uc.Create(context.Background(), agentchatfolder.CreateInput{
		ID: "f-fixed", WorkspaceID: workspaceID, Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, "f-fixed", created.ID)
}

func TestCreate_TrimsAndRefusesABlankName(t *testing.T) {
	_, _, uc := newUsecase(t)
	ctx := context.Background()

	created, _, err := uc.Create(ctx, agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, Name: "  spikes  ",
	})
	require.NoError(t, err)
	assert.Equal(t, "spikes", created.Name)

	_, _, err = uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "   "})
	assert.ErrorIs(t, err, agentchatfolder.ErrNameRequired)
}

// A folder INSIDE a chat is the case that makes this tree different from the
// sidebar's: it holds no turns, so it can order a chat's threads without ever
// being mistaken for one.
func TestCreate_NestsInsideAChat(t *testing.T) {
	folders, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)

	created, _, err := uc.Create(context.Background(), agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, ParentID: "c1", Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, "c1", created.ParentID)
	assert.Equal(t, "c1", folderRow(t, folders, created.ID).ParentID)
}

func TestCreate_RefusesAParentThatDoesNotExist(t *testing.T) {
	_, _, uc := newUsecase(t)

	_, _, err := uc.Create(context.Background(), agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, ParentID: "nowhere", Name: "spikes",
	})
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

// A folder in another workspace is reported as a cross-workspace edge rather
// than as a missing row, because the two are fixed in different ways.
func TestCreate_RefusesAFolderParentInAnotherWorkspace(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(folders.Rows, domain.ChatFolder{ID: "f-other", WorkspaceID: "ws-2"})

	_, _, err := uc.Create(context.Background(), agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, ParentID: "f-other", Name: "spikes",
	})
	assert.ErrorIs(t, err, agentchatfolder.ErrCrossWorkspace)
}

// A chat in another workspace is the same refusal, and it matters more: a chat
// parent is what the row READS, so accepting it would let an agent inherit
// context from a workspace the user is not in.
func TestCreate_RefusesAChatParentInAnotherWorkspace(t *testing.T) {
	_, chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", WorkspaceID: "ws-2"})

	_, _, err := uc.Create(context.Background(), agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, ParentID: "c-other", Name: "spikes",
	})
	assert.ErrorIs(t, err, agentchatfolder.ErrCrossWorkspace)
}

// The keyed chat read heals the chat read model for the one id it is asked
// about; the workspace list only heals a model that is entirely empty. So a
// parent this workspace's list did not carry can still be a legitimate
// container, and refusing it would reject a drop onto a chat the user can see.
func TestCreate_AcceptsAChatTheWorkspaceListDidNotCarry(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.MissingID = "c1"

	created, _, err := uc.Create(context.Background(), agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, ParentID: "c1", Name: "spikes",
	})
	require.NoError(t, err)
	assert.Equal(t, "c1", created.ParentID)
}

func TestCreate_SurfacesASnapshotFailure(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.FindErr = errors.New("boom")

	_, _, err := uc.Create(context.Background(), agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, Name: "spikes",
	})
	assert.ErrorContains(t, err, "boom")
}

func TestCreate_SurfacesAChatListFailure(t *testing.T) {
	_, chats, uc := newUsecase(t)
	chats.ListErr = errors.New("chats down")

	_, _, err := uc.Create(context.Background(), agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, Name: "spikes",
	})
	assert.ErrorContains(t, err, "chats down")
}

func TestCreate_SurfacesAParentLookupFailure(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.FindByKeyErr = errors.New("key read down")

	_, _, err := uc.Create(context.Background(), agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, ParentID: "nowhere", Name: "spikes",
	})
	assert.ErrorContains(t, err, "key read down")
}

func TestCreate_SurfacesASaveFailure(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.SaveErr = errors.New("disk full")

	_, _, err := uc.Create(context.Background(), agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, Name: "spikes",
	})
	assert.ErrorContains(t, err, "disk full")
}

// A folder create renumbers the chats already at that level and moves none of
// them, so the chat write it can fail on is the renumber.
func TestCreate_SurfacesAChatRenumberFailure(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	chats.OrderErr = errors.New("aggregate down")

	_, _, err := uc.Create(context.Background(), agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, Name: "spikes",
	})
	assert.ErrorContains(t, err, "aggregate down")
}

func TestListInWorkspace_ScopesToTheWorkspace(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(folders.Rows,
		domain.ChatFolder{ID: "f1", WorkspaceID: workspaceID},
		domain.ChatFolder{ID: "f2", WorkspaceID: "ws-2"},
	)

	rows, err := uc.ListInWorkspace(context.Background(), workspaceID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "f1", rows[0].ID)
}

func TestListInWorkspace_SurfacesAStoreFailure(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.FindErr = errors.New("boom")

	_, err := uc.ListInWorkspace(context.Background(), workspaceID)
	assert.ErrorContains(t, err, "boom")
}

func TestRename_TrimsAndRefusesABlankName(t *testing.T) {
	folders, _, uc := newUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "old"})
	require.NoError(t, err)

	renamed, err := uc.Rename(ctx, workspaceID, created.ID, "  new  ")
	require.NoError(t, err)
	assert.Equal(t, "new", renamed.Name)
	assert.Equal(t, "new", folderRow(t, folders, created.ID).Name)

	_, err = uc.Rename(ctx, workspaceID, created.ID, " ")
	assert.ErrorIs(t, err, agentchatfolder.ErrNameRequired)
}

// A folder addressed from the wrong workspace does not exist as far as that
// caller is concerned — any other answer would confirm a row it may not touch.
func TestRename_RefusesAFolderInAnotherWorkspace(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(folders.Rows, domain.ChatFolder{ID: "f-other", WorkspaceID: "ws-2"})

	_, err := uc.Rename(context.Background(), workspaceID, "f-other", "new")
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestRename_SurfacesAReadFailure(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.FindByKeyErr = errors.New("boom")

	_, err := uc.Rename(context.Background(), workspaceID, "f1", "new")
	assert.ErrorContains(t, err, "boom")
}

func TestRename_SurfacesASaveFailure(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(folders.Rows, domain.ChatFolder{ID: "f1", WorkspaceID: workspaceID})
	folders.SaveErr = errors.New("disk full")

	_, err := uc.Rename(context.Background(), workspaceID, "f1", "new")
	assert.ErrorContains(t, err, "disk full")
}

// Both levels are left dense: the one the row joined, and the one it left.
func TestMove_DensifiesBothLevels(t *testing.T) {
	_, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	moved, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "spikes"})
	require.NoError(t, err)

	placed, shifted, err := uc.Move(ctx, workspaceID, moved.ID, agentchatfolder.MoveInput{
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
	_, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	created, _, err := uc.Create(ctx, agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, ParentID: "c1", Name: "spikes",
	})
	require.NoError(t, err)

	placed, _, err := uc.Move(ctx, workspaceID, created.ID, agentchatfolder.MoveInput{})
	require.NoError(t, err)
	assert.Equal(t, "c1", placed.ParentID)
	assert.Equal(t, 0, placed.Order)
}

// An explicit index reorders WITHIN one level and reports every sibling the
// renumber moved, so no client is left holding stale orders.
func TestMove_ReordersWithinALevelAndReportsTheCollateral(t *testing.T) {
	_, _, uc := newUsecase(t)
	ctx := context.Background()
	first, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "a"})
	require.NoError(t, err)
	second, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "b"})
	require.NoError(t, err)

	placed, shifted, err := uc.Move(ctx, workspaceID, second.ID, agentchatfolder.MoveInput{Order: index(0)})
	require.NoError(t, err)
	assert.Equal(t, 0, placed.Order)
	require.Len(t, shifted, 1)
	assert.Equal(t, first.ID, shifted[0].ID)
	assert.Equal(t, 1, shifted[0].Order)
}

func TestMove_RefusesAFolderOntoItself(t *testing.T) {
	_, _, uc := newUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "spikes"})
	require.NoError(t, err)

	_, _, err = uc.Move(ctx, workspaceID, created.ID, agentchatfolder.MoveInput{ParentID: name(created.ID)})
	assert.ErrorIs(t, err, agentchatfolder.ErrCycle)
}

// A move into a folder's own subtree would leave a set of rows unreachable from
// the panel root: they exist, nothing renders them, and nothing can drag them
// back out.
func TestMove_RefusesAMoveIntoItsOwnSubtree(t *testing.T) {
	_, _, uc := newUsecase(t)
	ctx := context.Background()
	outer, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "outer"})
	require.NoError(t, err)
	inner, _, err := uc.Create(ctx, agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, ParentID: outer.ID, Name: "inner",
	})
	require.NoError(t, err)

	_, _, err = uc.Move(ctx, workspaceID, outer.ID, agentchatfolder.MoveInput{ParentID: name(inner.ID)})
	assert.ErrorIs(t, err, agentchatfolder.ErrCycle)
}

func TestMove_RefusesAFolderInAnotherWorkspace(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(folders.Rows, domain.ChatFolder{ID: "f-other", WorkspaceID: "ws-2"})

	_, _, err := uc.Move(context.Background(), workspaceID, "f-other", agentchatfolder.MoveInput{})
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestMove_SurfacesASnapshotFailure(t *testing.T) {
	_, chats, uc := newUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "spikes"})
	require.NoError(t, err)
	chats.ListErr = errors.New("chats down")

	_, _, err = uc.Move(ctx, workspaceID, created.ID, agentchatfolder.MoveInput{})
	assert.ErrorContains(t, err, "chats down")
}

func TestMove_RefusesAParentThatDoesNotExist(t *testing.T) {
	_, _, uc := newUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "spikes"})
	require.NoError(t, err)

	_, _, err = uc.Move(ctx, workspaceID, created.ID, agentchatfolder.MoveInput{ParentID: name("nowhere")})
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestMove_SurfacesASaveFailure(t *testing.T) {
	folders, _, uc := newUsecase(t)
	ctx := context.Background()
	_, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "a"})
	require.NoError(t, err)
	second, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "b"})
	require.NoError(t, err)
	folders.SaveErr = errors.New("disk full")

	_, _, err = uc.Move(ctx, workspaceID, second.ID, agentchatfolder.MoveInput{Order: index(0)})
	assert.ErrorContains(t, err, "disk full")
}

// A folder holds no conversation, so what it held outlives it. This is the
// opposite of deleting a CHAT — see the cascade tests below.
func TestDelete_PromotesChildrenToTheFoldersOwnParent(t *testing.T) {
	folders, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	outer, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "outer"})
	require.NoError(t, err)
	inner, _, err := uc.Create(ctx, agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, ParentID: outer.ID, Name: "inner",
	})
	require.NoError(t, err)
	_, _, err = uc.PlaceChat(ctx, workspaceID, "c1", agentchatfolder.PlaceInput{ParentID: name(outer.ID)})
	require.NoError(t, err)

	written, err := uc.Delete(ctx, workspaceID, outer.ID)
	require.NoError(t, err)

	assert.Equal(t, "", folderRow(t, folders, inner.ID).ParentID, "the child folder rises to the root")
	assert.Equal(t, "", chatRow(t, chats, "c1").ParentID, "the chat survives its folder")
	ids := make([]string, 0, len(written))
	for _, row := range written {
		ids = append(ids, row.ID)
	}
	assert.Equal(t, []string{inner.ID}, ids, "the promoted folder rows come back for broadcast")
}

func TestDelete_RefusesAFolderInAnotherWorkspace(t *testing.T) {
	folders, _, uc := newUsecase(t)
	folders.Rows = append(folders.Rows, domain.ChatFolder{ID: "f-other", WorkspaceID: "ws-2"})

	_, err := uc.Delete(context.Background(), workspaceID, "f-other")
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestDelete_SurfacesASnapshotFailure(t *testing.T) {
	_, chats, uc := newUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "spikes"})
	require.NoError(t, err)
	chats.ListErr = errors.New("chats down")

	_, err = uc.Delete(ctx, workspaceID, created.ID)
	assert.ErrorContains(t, err, "chats down")
}

func TestDelete_SurfacesARemovalFailure(t *testing.T) {
	folders, _, uc := newUsecase(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "spikes"})
	require.NoError(t, err)
	folders.DeleteErr = errors.New("locked")

	_, err = uc.Delete(ctx, workspaceID, created.ID)
	assert.ErrorContains(t, err, "locked")
}

// A chat's parent IS its context lineage, so this write legitimately turns a
// standalone chat into a thread of another and back.
func TestPlaceChat_RewritesLineage(t *testing.T) {
	_, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)

	placed, _, err := uc.PlaceChat(ctx, workspaceID, "c2", agentchatfolder.PlaceInput{ParentID: name("c1")})
	require.NoError(t, err)
	assert.Equal(t, "c1", placed.ParentID)
	assert.Equal(t, 0, placed.Order)
	assert.Equal(t, "c1", chatRow(t, chats, "c2").ParentID)

	back, _, err := uc.PlaceChat(ctx, workspaceID, "c2", agentchatfolder.PlaceInput{ParentID: name("")})
	require.NoError(t, err)
	assert.Equal(t, "", back.ParentID)
}

// Chats and folders share one level, so a chat drop renumbers the folders in it
// and those rows have to come back for broadcast.
func TestPlaceChat_ReturnsTheFoldersItShifted(t *testing.T) {
	_, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	folder, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "spikes"})
	require.NoError(t, err)
	require.Equal(t, 1, folder.Order)

	_, shifted, err := uc.PlaceChat(ctx, workspaceID, "c1", agentchatfolder.PlaceInput{Order: index(1)})
	require.NoError(t, err)
	require.Len(t, shifted, 1)
	assert.Equal(t, folder.ID, shifted[0].ID)
	assert.Equal(t, 0, shifted[0].Order, "the folder took the slot the chat left")
}

func TestPlaceChat_WithNothingRequestedKeepsThePlacement(t *testing.T) {
	_, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)

	placed, _, err := uc.PlaceChat(ctx, workspaceID, "c2", agentchatfolder.PlaceInput{})
	require.NoError(t, err)
	assert.Equal(t, "c1", placed.ParentID)
}

func TestPlaceChat_RefusesAChatOntoItself(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		agentchatfolder.PlaceInput{ParentID: name("c1")})
	assert.ErrorIs(t, err, agentchatfolder.ErrCycle)
}

// A chat inside its own subtree is worse than unreachable: its context walk
// would never terminate at the root.
func TestPlaceChat_RefusesAMoveIntoItsOwnThread(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		agentchatfolder.PlaceInput{ParentID: name("c2")})
	assert.ErrorIs(t, err, agentchatfolder.ErrCycle)
}

func TestPlaceChat_RefusesAChatFromAnotherWorkspace(t *testing.T) {
	_, chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", WorkspaceID: "ws-2"})

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c-other", agentchatfolder.PlaceInput{})
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestPlaceChat_RefusesAnUnknownChat(t *testing.T) {
	_, _, uc := newUsecase(t)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "nowhere", agentchatfolder.PlaceInput{})
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestPlaceChat_SurfacesASnapshotFailure(t *testing.T) {
	folders, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	folders.FindErr = errors.New("boom")

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1", agentchatfolder.PlaceInput{})
	assert.ErrorContains(t, err, "boom")
}

// A reorder inside one level writes indices and no parents, so the write it can
// fail on is the renumber — for the subject as much as for the rows it passed.
func TestPlaceChat_SurfacesARenumberWriteFailure(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	chats.OrderErr = errors.New("aggregate down")

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		agentchatfolder.PlaceInput{Order: index(0)})
	assert.ErrorContains(t, err, "aggregate down")
}

func TestPlaceChat_SurfacesAPlacementWriteFailure(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	chats.SetErr = errors.New("aggregate down")

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		agentchatfolder.PlaceInput{ParentID: name("c1")})
	assert.ErrorContains(t, err, "aggregate down")
}

// The cascade, and the reason it exists: a thread reads its parent's turns, so
// leaving it behind would strand a conversation whose whole premise is gone.
// Deepest first, so no intermediate state has a chat pointing at a parent that
// has already been erased.
func TestDeleteChat_TakesTheWholeSubtreeDeepestFirst(t *testing.T) {
	_, chats, uc := newUsecase(t)
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
// chat gone it has nothing left to order, so it goes too — and it is a plain
// row, so the ids come back for the caller to broadcast.
func TestDeleteChat_TakesTheFoldersCaughtInTheSubtree(t *testing.T) {
	folders, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "root", 1)
	inside, _, err := uc.Create(ctx, agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, ParentID: "root", Name: "spikes",
	})
	require.NoError(t, err)
	seedThread(chats, "filed", inside.ID, 2)

	removed, err := uc.DeleteChat(ctx, "root")
	require.NoError(t, err)

	assert.Equal(t, []string{"filed", "root"}, removed.Chats)
	assert.Equal(t, []string{inside.ID}, removed.Folders)
	assert.Empty(t, folders.Rows, "the folder went with the chat that held it")
}

// The level the deleted chat left is renumbered, and the folders that moved come
// back so no client holds a stale order.
func TestDeleteChat_DensifiesTheLevelItLeft(t *testing.T) {
	_, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	folder, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "spikes"})
	require.NoError(t, err)
	require.Equal(t, 1, folder.Order)

	removed, err := uc.DeleteChat(ctx, "c1")
	require.NoError(t, err)

	require.Len(t, removed.Shifted, 1)
	assert.Equal(t, folder.ID, removed.Shifted[0].ID)
	assert.Equal(t, 0, removed.Shifted[0].Order)
}

func TestDeleteChat_RefusesAnUnknownChat(t *testing.T) {
	_, _, uc := newUsecase(t)

	_, err := uc.DeleteChat(context.Background(), "nowhere")
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestDeleteChat_SurfacesASnapshotFailure(t *testing.T) {
	folders, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	folders.FindErr = errors.New("boom")

	_, err := uc.DeleteChat(context.Background(), "c1")
	assert.ErrorContains(t, err, "boom")
}

func TestDeleteChat_SurfacesAPurgeFailure(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.PurgeErr = errors.New("cli wedged")

	_, err := uc.DeleteChat(context.Background(), "c1")
	assert.ErrorContains(t, err, "cli wedged")
}

func TestDeleteChat_SurfacesAFolderRemovalFailure(t *testing.T) {
	folders, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	_, _, err := uc.Create(ctx, agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, ParentID: "c1", Name: "spikes",
	})
	require.NoError(t, err)
	folders.DeleteErr = errors.New("locked")

	_, err = uc.DeleteChat(ctx, "c1")
	assert.ErrorContains(t, err, "locked")
}

func TestDeleteChat_SurfacesADensifyWriteFailure(t *testing.T) {
	folders, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	_, _, err := uc.Create(ctx, agentchatfolder.CreateInput{WorkspaceID: workspaceID, Name: "spikes"})
	require.NoError(t, err)
	folders.SaveErr = errors.New("disk full")

	_, err = uc.DeleteChat(ctx, "c1")
	assert.ErrorContains(t, err, "disk full")
}

// A chat the snapshot's list did not carry is still deleted: the keyed read is
// the authority, and the densify simply counts the rows that were there.
func TestDeleteChat_ProceedsWhenTheListLagsTheAggregate(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.MissingID = "c1"

	removed, err := uc.DeleteChat(context.Background(), "c1")
	require.NoError(t, err)
	assert.Equal(t, []string{"c1"}, removed.Chats)
	assert.Equal(t, []string{"c1"}, chats.Purged)
}
