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

const (
	workspaceID = "ws-1"
	repoID      = "repo-1"
)

// errNoLog stands in for the event log being unreachable.
var errNoLog = errors.New("log unavailable")

// seedFolder creates a folder hanging off parentID through the SAME public
// Create verb every other test's fixtures go through, scoped to the module's
// single-repo fixture (repoID, "repo-1") — every test in this file that
// nests folders inside one another lives in that one repo's world. A folder
// is a domain.Folder+domain.Node pair now (2026-09-08
// sidebar-placement-unification Task 5 for home-scoped, Task 8 for
// repo-scoped too), never a Chat row — this is why seeding it now has to run
// through the usecase rather than appending straight onto chats.Rows.
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
// chat write into a renumber. c1's placement under "spikes" is Node-backed
// (2026-09-08 sidebar-placement-unification Task 8), so the promotion write
// this test pins lands on Nodes.SetPlacement, not the chat aggregate.
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

// folderRow reads id's current Chat-shaped view back through the public
// ListInRepo(repoID) — a folder's identity/position live on domain.Folder/
// domain.Node now, never on chats.Rows, so this is the one door left to read
// it through.
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

// folderRows is every folder row repoID's own tree still holds.
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
	assert.Equal(t, "c1", folderRow(t, uc, created.ID).ParentID)
}

func TestCreate_RefusesAParentThatDoesNotExist(t *testing.T) {
	_, uc := newUsecase(t)

	_, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "nowhere", Name: "spikes",
	})
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

// Stage 3 (the folder-scoping golden rule) has landed: a chat parent in
// ANOTHER repo is now refused, the same way ErrCrossWorkspace already refuses
// a cross-workspace CHAT parent. This is the "conscious assertion change" the
// deleted permissive-pin test's own doc comment asked for, not a silent
// regression — caught live as a folder left on top of the wrong repo after a
// cross-repo drag.
func TestCreate_RefusesAChatParentFromAnotherRepo(t *testing.T) {
	chats, uc := newUsecase(t)
	// ws-2 is never registered against repoID in newUsecaseWithWork's fixture
	// — RepoOf answers "" for it, a different repo scope than repoID.
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", Type: domain.ChatTypeChat, WorkspaceID: "ws-2"})

	_, _, err := uc.Create(context.Background(), tree.CreateInput{
		RepoID: repoID, ParentID: "c-other", Name: "spikes",
	})
	assert.ErrorIs(t, err, tree.ErrCrossRepo)
}

// The same chat parent, now in a workspace the fixture's own repo actually
// owns — accepted, same as TestCreate_NestsInsideAChat's folder-owned parent.
func TestCreate_AcceptsAChatParentFromTheSameRepo(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", Type: domain.ChatTypeChat, WorkspaceID: workspaceID})

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

// A folder's identity is a single Folders.Save now — its own equivalent of
// the OLD two-step mint-then-name (Chats.Create then SetTitle) collapses
// into one write, since domain.Folder carries the name directly. Both
// halves of that OLD two-step failure surface (TestCreate_SurfacesACreateFailure/
// TestCreate_SurfacesATitleFailure) now cover the SAME call — kept as two
// tests, mirroring the old suite's own shape, rather than merged into one.
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

// A folder create renumbers the chats already at that level and moves none of
// them, so the chat write it can fail on is the renumber.
// A create that mints and names the folder successfully but fails during the
// densify that follows must not leave that row behind: the user was told the
// create failed, and CreateChat's own discard (chats.go) sets the precedent
// this mirrors — the whole post-mint sequence is covered, not just naming.
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

// The failure covered above is a SIBLING's renumber; this is the new folder's
// OWN placement write failing instead — discard must cover both call shapes
// persist can take. The new folder's own placement is ALWAYS a Node mint
// (its own row is freshIDs by construction), so the failure this test
// injects is nodes.CreateErr now, not chats.SetErr.
func TestCreate_DiscardsTheFolderWhenItsOwnPlacementWriteFails(t *testing.T) {
	_, folders, nodes, uc, _ := newUsecaseWithStores(t)
	nodes.CreateErr = errors.New("wedged")

	_, _, err := uc.Create(context.Background(), tree.CreateInput{ID: "f-new", RepoID: repoID, Name: "spikes"})
	assert.ErrorContains(t, err, "wedged")
	assert.Empty(t, folders.Saved, "the half-created folder's identity row must be discarded too")
	assert.Empty(t, nodes.Rows, "and its Node row (though Create never succeeding means Forget is a no-op)")
}

// ListInRepo filters to folder-typed rows.
func TestListInRepo_ReturnsOnlyFolderTypedRows(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(t, uc, "f1", "")
	seedChat(chats, "c1", 1)

	rows, err := uc.ListInRepo(context.Background(), repoID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "f1", rows[0].ID)
}

// The repo boundary IS enforced now (Folders.FindAll itself still returns
// every folder across every repo AND home): a folder that belongs to a
// DIFFERENT repo, or to project-home ("" — a distinct scope of its own,
// never a wildcard), never bleeds into another repo's list. Caught live as a
// folder left "on top of" the wrong repo. A folder is a domain.Folder row
// now (2026-09-08 sidebar-placement-unification Task 5 for home-scoped,
// Task 8 for repo-scoped too), never a ChatTypeFolder Chat row — seeded here
// through Folders.Save directly (the OTHER repo's folder never goes through
// this package's own Create, which always scopes to THIS test's repoID).
func TestListInRepo_IsolatesByRepo(t *testing.T) {
	_, folders, _, uc, _ := newUsecaseWithStores(t)
	seedFolder(t, uc, "f-this-repo", "")
	require.NoError(t, folders.Save(context.Background(),
		domain.Folder{ID: "f-other-repo", Name: "theirs", RepoID: "repo-2"}))

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

// Rename resolves id through the Folders store now, not Chats.LoadChat — a
// folder's identity lives there, home-scoped or repo-scoped alike.
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

// Both levels are left dense: the one the row joined, and the one it left.
func TestMove_DensifiesBothLevels(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	// The destination is another repo-root FOLDER, not a chat — a chat
	// carries a real WorkspaceID (its owning branch's), so filing "moved"
	// under one would cross from the bare repo-root context into that
	// branch's own (checkFolderContextMove's golden rule, the finer half:
	// "context" is Project -> Repo -> Locked branch -> Parent unlocked, and
	// the repo root is its own context, distinct from every branch in it).
	// "container" is created BEFORE "moved" so removing "moved" (the LAST
	// root sibling) never has anyone to renumber — same "shifted is empty"
	// shape the test always asserted, now for a reason that has nothing to
	// do with what is under test here.
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
// can surface is the order write. Folders are Node-backed now (2026-09-08
// sidebar-placement-unification Task 5 for home-scoped, Task 8 for
// repo-scoped too), so the injected failure is nodes.OrderErr.
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

// A move that crosses into a different container writes the subject's
// placement whole — nodes.PlaceErr now, not the chat aggregate's.
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

	assert.Equal(t, "", folderRow(t, uc, inner.ID).ParentID, "the child folder rises to the root")
	assert.Equal(t, "", chatRow(t, chats, "c1").ParentID, "the chat survives its folder")
	ids := make([]string, 0, len(written))
	for _, row := range written {
		ids = append(ids, row.ID)
	}
	assert.Equal(t, []string{inner.ID}, ids, "the promoted folder rows come back for broadcast")
}

// A folder delete PROMOTES what it held, which still moves every row one level
// up — and a working row does not move, unconditionally, whichever verb is
// asking (backend addendum §2, invariant 9). This is the same refusal
// TestMove_RefusesWorkingSubtree pins for Move, over the same guard, on the
// verb that used to skip it.
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

// A folder's identity row lives in Folders now (2026-09-08
// sidebar-placement-unification Task 5 for home-scoped, Task 8 for
// repo-scoped too), so the failure this test injects is folders.DeleteErr,
// not chats.ForgetErr.
func TestDelete_SurfacesARemovalFailure(t *testing.T) {
	_, folders, _, uc, _ := newUsecaseWithStores(t)
	ctx := context.Background()
	created, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)
	folders.DeleteErr = errors.New("locked")

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
	assert.Empty(t, folderRows(t, uc), "the folder went with the chat that held it")
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

// A chat delete takes its whole subtree with it, so a working row anywhere
// below the root refuses the whole delete — before a single row is purged.
// The refusal has NO bypass: unlike a locked-branch refusal, a working chat
// is never overridable here.
func TestDeleteChat_RefusesWorkingSubtree(t *testing.T) {
	chats, uc, work := newUsecaseWithWork(t)
	ctx := context.Background()
	seedChat(chats, "root", 1)
	seedThread(chats, "child", "root", 2)
	work.Set("child", true)

	_, err := uc.DeleteChat(ctx, "root")
	assert.ErrorIs(t, err, tree.ErrSubtreeWorking)
	assert.Empty(t, chats.Purged, "nothing may be torn down once any row in the subtree refuses")
}

// The most literal reading of "unconditional": the chat NAMED by the delete
// is itself working, with no descendants at all. subtreeIDsOf's walk has to
// include the root it is handed, or a leaf chat with a live turn could be
// erased out from under it.
func TestDeleteChat_RefusesTheNamedChatItselfWhenWorking(t *testing.T) {
	chats, uc, work := newUsecaseWithWork(t)
	ctx := context.Background()
	seedChat(chats, "solo", 1)
	work.Set("solo", true)

	_, err := uc.DeleteChat(ctx, "solo")
	assert.ErrorIs(t, err, tree.ErrSubtreeWorking)
	assert.Empty(t, chats.Purged, "the working leaf itself must never be purged")
}

// A row working OUTSIDE the deleted subtree is none of this delete's business:
// the guard only watches the rows the cascade is about to take.
func TestDeleteChat_IdleSubtreeCascadesDespiteAWorkingBystander(t *testing.T) {
	chats, uc, work := newUsecaseWithWork(t)
	ctx := context.Background()
	seedChat(chats, "root", 1)
	seedThread(chats, "child", "root", 2)
	seedChat(chats, "bystander", 3)
	work.Set("bystander", true)

	removed, err := uc.DeleteChat(ctx, "root")
	require.NoError(t, err)
	assert.Equal(t, []string{"child", "root"}, removed.Chats)
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

// Regression, reported live as "Couldn't remove Untitled chat: agent chat
// folder: purge chat ...: agentchat: not found" while removing a parent
// that had children. Root cause: a child can be real at the TREE level (it
// has a parent, an order, it renders) while never having minted a
// conversation aggregate at all — a thread whose create never got past
// placement — so purging it answers apperr.ErrNotFound. purgeAll used to
// fail the WHOLE cascade on that single not-found, deleting nothing at all,
// parent included. It must tolerate a not-found the same way the sibling
// worktree-reaping walk already tolerates one on DiscardChildWorkspace, and
// keep going.
func TestDeleteChat_ToleratesAnAlreadyGoneDescendantAndStillDeletesTheRest(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "root", 1)
	seedThread(chats, "ghost", "root", 2)
	seedThread(chats, "grandchild", "ghost", 3)
	chats.PurgeNotFoundID = "ghost"

	removed, err := uc.DeleteChat(ctx, "root")

	require.NoError(t, err)
	assert.Equal(t, []string{"grandchild", "ghost", "root"}, chats.Purged,
		"the not-found descendant does not stop the cascade around it")
	assert.Equal(t, []string{"grandchild", "ghost", "root"}, removed.Chats)
	assert.Empty(t, chats.Rows, "every row in the subtree is gone, ghost included")
}

// A folder caught in the cascade is erased through Folders now (2026-09-08
// sidebar-placement-unification Task 5 for home-scoped, Task 8 for
// repo-scoped too), so the failure this test injects is folders.DeleteErr,
// not chats.ForgetErr.
func TestDeleteChat_SurfacesAFolderRemovalFailure(t *testing.T) {
	chats, folders, _, uc, _ := newUsecaseWithStores(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	_, _, err := uc.Create(ctx, tree.CreateInput{
		RepoID: repoID, ParentID: "c1", Name: "spikes",
	})
	require.NoError(t, err)
	folders.DeleteErr = errors.New("locked")

	_, err = uc.DeleteChat(ctx, "c1")
	assert.ErrorContains(t, err, "locked")
}

// "spikes" is a folder sibling sharing c1's own root level, so ITS densify
// write (once c1 is purged) is a Nodes.SetOrder now, not the chat
// aggregate's.
func TestDeleteChat_SurfacesADensifyWriteFailure(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	_, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)
	nodes.OrderErr = errors.New("disk full")

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
