package tree_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestDeleteChat_RefusesAFolderID pins the asymmetry this package's own doc
// calls "the whole domain rule": deleting a CHAT cascades into its threads,
// deleting a FOLDER promotes what it held. A folder id routed through the CHAT
// verb — legal at the repo-scoped mount, where :wsId is absent and the
// workspace comparison no longer separates the two — used to run the cascade,
// erasing every chat filed in that folder. It is now refused as not-found,
// mirroring plan.go's load() refusing a chat id through the FOLDER verb.
func TestDeleteChat_RefusesAFolderID(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(chats, "spikes", "")
	seedThread(chats, "c1", "spikes", 1)

	_, err := uc.DeleteChat(context.Background(), "spikes")

	require.ErrorIs(t, err, apperr.ErrNotFound)
	assert.Empty(t, chats.Purged, "a folder id must never reach the chat cascade")
}

// TestPlaceChat_RefusesAFolderID is the same guard on the placement half: a
// folder moves through Move, which densifies its own level and broadcasts a
// folder frame; PlaceChat would move it as a chat and announce nothing.
func TestPlaceChat_RefusesAFolderID(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(chats, "spikes", "")
	seedFolder(chats, "ideas", "")

	_, _, err := uc.PlaceChat(context.Background(), "", "spikes", tree.PlaceInput{ParentID: name("ideas")})

	require.ErrorIs(t, err, apperr.ErrNotFound)
	assert.Equal(t, "", folderRow(t, chats, "spikes").ParentID, "nothing may have moved")
}

// TestPlaceChat_ABubbleDoesNotSeeEveryFolderTwice pins the snapshot a BUBBLE
// (WorkspaceID == "") plans against. workspaceSnapshotAround reads
// ListByWorkspace and then appends every folder from ListChats, because a
// folder carries no workspace of its own and would otherwise be missing from
// the level a chat is densifying. For workspaceID == "" that first read ALREADY
// returns every folder — they match the empty workspace exactly — so every
// folder arrived twice and the plan counted a sibling space with twice the
// rows it has. NextSlot is where that surfaces: the bubble lands past the end
// of a level it should have joined at index 2.
func TestPlaceChat_ABubbleDoesNotSeeEveryFolderTwice(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(chats, "f1", "")
	seedFolder(chats, "f2", "")
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: "b1", Type: domain.ChatTypeChat, ParentID: "f1",
	})

	_, _, err := uc.PlaceChat(context.Background(), "", "b1", tree.PlaceInput{ParentID: name("")})
	require.NoError(t, err)

	assert.Equal(t, 2, chatRow(t, chats, "b1").Order,
		"the root holds two folders, so a bubble joining it takes slot 2")
}

// A chat with nowhere in particular to go takes the unplaced spawn, untouched.
// There is no edge to write, so there is no gap to open between minting and
// starting, and a plain new chat must be created in exactly the order it always
// was.
func TestCreateChat_AtTheRootTakesTheUnplacedSpawn(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	chatID, runnerID, err := uc.CreateChat(context.Background(), workspaceID, "claude", "", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	assert.Equal(t, "runner-c-new", runnerID)
	assert.Equal(t, []string{"claude"}, chats.Spawned)
	assert.Empty(t, chats.Minted, "the split create is for a chat that has somewhere to be placed")
}

// THE ORDERING. A chat created under another chat must carry the parent edge
// before its CLI is started, because the spawn is what tells a thread what it
// reads — a chat placed afterwards spends its whole first session, the one the
// user just asked for, believing it is a standalone chat.
//
// ParentAtStart is read inside StartRunner, so it fails if the placement moves
// after the start. The end state is identical either way, which is exactly why
// asserting the end state would prove nothing.
func TestCreateChat_PlacesTheChatBeforeStartingItsCLI(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"

	chatID, runnerID, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	assert.Equal(t, "runner-c-new", runnerID)
	assert.Equal(t, []mocks.StartCall{{
		ChatID:        "c-new",
		ProviderID:    "claude",
		ParentAtStart: "c1",
	}}, chats.Started,
		"the CLI must come up on a chat that is ALREADY a thread, or its first session inherits nothing")
}

// A folder parent is "new chat in this folder" and takes the identical path.
func TestCreateChat_InAFolderPlacesItThereToo(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(chats, "spikes", "")
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "spikes", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	require.NoError(t, err)
	assert.Equal(t, "spikes", chatRow(t, chats, "c-new").ParentID)
}

// A new chat lands at the END of its parent's sibling space, the same rule a new
// folder follows — the placement usecase's own, not a second copy of it.
func TestCreateChat_LandsAtTheEndOfItsParentsSiblingSpace(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	require.NoError(t, err)
	assert.Equal(t, 1, chatRow(t, chats, "c-new").Order)
}

// A chat BORN under a parent gets no "this chat was moved" note: it was not
// moved, and it has nothing above the line for such a note to date. The note is
// suppressed by the ledger being empty, which is the agent usecase's call — here
// we only prove the create routes through the same PlaceChat every drag does.
func TestCreateChat_StillGoesThroughThePlacementPath(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	require.NoError(t, err)
	assert.Positive(t, chats.SetCalls, "the placement is written through the chat aggregate, like any other")
}

// A parent that does not exist is refused before anything is minted or spawned.
// Minting first and cleaning up afterwards would make a create and a delete out
// of every mistyped id.
func TestCreateChat_RefusesAnUnknownParentWithoutMintingAnything(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "nowhere", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	require.Error(t, err)
	assert.Empty(t, chats.Minted)
	assert.Empty(t, chats.Started)
	assert.Empty(t, chats.Spawned)
}

// A folder carries no workspace of its own, so it is always an acceptable
// parent — unlike a CHAT parent, which still enforces the workspace boundary
// below. Enforcing a repo boundary on folders too is stage 3's walk.
func TestCreateChat_AcceptsAFolderParentRegardlessOfProvenance(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(chats, "f-other", "")
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "f-other", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	assert.NoError(t, err)
}

// A chat parent in another workspace is refused the same way.
func TestCreateChat_RefusesAChatParentInAnotherWorkspace(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", WorkspaceID: "ws-2"})

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c-other", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	assert.ErrorIs(t, err, tree.ErrCrossWorkspace)
	assert.Empty(t, chats.Minted)
}

func TestCreateChat_SurfacesASnapshotFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.ListErr = errors.New("folders down")

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	assert.ErrorContains(t, err, "folders down")
	assert.Empty(t, chats.Minted)
}

func TestCreateChat_SurfacesAMintFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.MintErr = errors.New("mint down")

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	assert.ErrorContains(t, err, "mint down")
	assert.Empty(t, chats.Purged, "nothing was minted, so there is nothing to take back")
}

// A create the user was told FAILED must not leave a chat behind. Everything past
// the mint therefore takes the chat back out again.
func TestCreateChat_TakesTheChatBackOutWhenThePlacementFails(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"
	chats.SetErr = errors.New("placement down")

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	assert.ErrorContains(t, err, "placement down")
	assert.Equal(t, []string{"c-new"}, chats.Purged)
	assert.Empty(t, chats.Started, "and no CLI is started on a chat that is about to be erased")
}

func TestCreateChat_TakesTheChatBackOutWhenTheCLIFailsToStart(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"
	chats.StartErr = errors.New("claude is not installed")

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	assert.ErrorContains(t, err, "claude is not installed")
	assert.Equal(t, []string{"c-new"}, chats.Purged)
}

// The purge is best-effort and never replaces the cause: the user asked to create
// a chat, and THAT is what failed. Reporting the cleanup's error instead would
// name a failure they cannot act on and hide the one they can.
func TestCreateChat_AFailedCleanupStillReportsTheOriginalFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"
	chats.StartErr = errors.New("claude is not installed")
	chats.PurgeErr = errors.New("purge down")

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	assert.ErrorContains(t, err, "claude is not installed")
	assert.NotErrorIs(t, err, apperr.ErrNotFound)
}

// CreateChat's ownWorktree branch (model spec §4.1/§5.1): mint and place the
// chat exactly as the plain-bubble path above does, then fill its workspace
// slot and start its CLI through agent.SpawnChatWithOwnWorktree instead of a
// bare StartRunner. workspaceID is passed as something OTHER than "" in every
// test below specifically to prove it is ignored: the row is a bubble until
// its own worktree exists, regardless of what the caller named.

// At the panel root there is nowhere to place the chat — the ordinary
// unplaced-spawn shortcut does not apply here, since ownWorktree still needs a
// row that exists (and is placed, when it has somewhere to go) before it can
// resolve a fork parent from it.
//
// This is an ORCHESTRATION test only: it proves CreateChat skips PlaceChat for
// parentID=="" and forwards exactly the right arguments to
// agent.SpawnChatWithOwnWorktree, using the fake Agent's unconditional
// success — it does not prove a root-level ownWorktree create succeeds in
// PRODUCTION. It never does: a chat with no parent has no ancestor to resolve
// a fork parent from, and the REAL SpawnChatWithOwnWorktree (own_worktree.go)
// refuses that with ErrNoForkParent, exactly as
// TestSpawnChatWithOwnWorktree_NoForkParent_Refuses (own_worktree_test.go)
// pins at the layer that actually resolves one.
func TestCreateChat_OwnWorktree_AtTheRootSkipsPlacementBeforeFillingTheSlot(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	chatID, runnerID, err := uc.CreateChat(context.Background(), "some-other-ws", "claude", "", tree.WorktreeSpec{Mode: tree.WorktreeFork})
	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	assert.Equal(t, "runner-c-new", runnerID)
	assert.Equal(t, []string{""}, chats.Minted, "the row is minted as a plain bubble, not into the caller's workspaceID")
	assert.Empty(t, chats.Placed, "there is nowhere to place a root-level chat")
	assert.Empty(t, chats.Spawned, "the ordinary unplaced SpawnChat is not the path ownWorktree takes")
	require.Len(t, chats.SpawnedOwnWorktree, 1)
	assert.Equal(t, "c-new", chats.SpawnedOwnWorktree[0].ChatID)
	assert.Equal(t, "claude", chats.SpawnedOwnWorktree[0].ProviderID)
}

// THE ORDERING, ownWorktree's own version of TestCreateChat_PlacesTheChatBeforeStartingItsCLI:
// a chat under another row must carry the parent edge before its fork parent is
// resolved and its CLI started, because ResolveForkParent — the walk
// SpawnChatWithOwnWorktree runs first — reads the row's OWN ParentID.
func TestCreateChat_OwnWorktree_PlacesTheChatBeforeFillingItsSlot(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(chats, "spikes", "")
	chats.NextID = "c-new"

	chatID, runnerID, err := uc.CreateChat(context.Background(), "some-other-ws", "claude", "spikes", tree.WorktreeSpec{Mode: tree.WorktreeFork})
	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	assert.Equal(t, "runner-c-new", runnerID)
	assert.Equal(t, "spikes", chatRow(t, chats, "c-new").ParentID)
	require.Len(t, chats.SpawnedOwnWorktree, 1)
	assert.Equal(t, "spikes", chats.SpawnedOwnWorktree[0].ParentAtStart,
		"the fork-parent walk must see the placement, or it resolves nothing")
}

// A folder parent takes the identical path, exactly as it does for a plain
// bubble thread.
func TestCreateChat_OwnWorktree_InAFolderPlacesItThereToo(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(chats, "spikes", "")
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), "", "claude", "spikes", tree.WorktreeSpec{Mode: tree.WorktreeFork})
	require.NoError(t, err)
	assert.Equal(t, "spikes", chatRow(t, chats, "c-new").ParentID)
}

// A parent that does not exist is refused before anything is minted or spawned,
// the same guarantee the plain-bubble path already gives.
func TestCreateChat_OwnWorktree_RefusesAnUnknownParentWithoutMintingAnything(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), "", "claude", "nowhere", tree.WorktreeSpec{Mode: tree.WorktreeFork})
	require.Error(t, err)
	assert.Empty(t, chats.Minted)
	assert.Empty(t, chats.SpawnedOwnWorktree)
}

// A chat parent that already OWNS a worktree in a different workspace is an
// acceptable fork point, the same way a BRANCH parent already is below: "under
// a row that carries a branch [or, since this widening, a worktree] the new
// row is a worktree" (model spec §4.1). This is the regular-forked-workspace
// case — a chat parent's own WorkspaceID can never equal the new chat's (it is
// "" here, since the new chat's workspace does not exist yet), so the
// cross-workspace refusal below must not fire for THIS caller, even though it
// still must for an ordinary thread naming the same parent (see
// TestCreateChat_RefusesAChatParentInAnotherWorkspace).
func TestCreateChat_OwnWorktree_AcceptsAWorktreeOwningChatParentInAnotherWorkspace(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", Type: domain.ChatTypeChat, WorkspaceID: "ws-2"})
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), "", "claude", "c-other", tree.WorktreeSpec{Mode: tree.WorktreeFork})
	assert.NoError(t, err)
	assert.Equal(t, "c-other", chatRow(t, chats, "c-new").ParentID)
}

// The bypass above is keyed on THIS BEING AN OWN-WORKTREE CREATION, not on the
// new chat's workspaceID happening to be "". An ordinary (non-ownWorktree)
// thread can ALSO be created with workspaceID=="" (a workspace-less bubble
// thread — see handlers.Create's own doc comment), and for THAT caller the
// cross-workspace refusal must still fire: a thread inherits its parent's cwd,
// and a thread claiming no workspace of its own placed under a chat that owns
// a DIFFERENT one is exactly the meaningless case ErrCrossWorkspace exists to
// catch. A fix that bypassed the check whenever workspaceID == "" — rather
// than whenever the caller is actually creating an own-worktree chat — would
// wrongly let this one through too.
func TestCreateChat_RefusesAChatParentInAnotherWorkspaceEvenWithNoWorkspaceOfItsOwn(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", Type: domain.ChatTypeChat, WorkspaceID: "ws-2"})

	_, _, err := uc.CreateChat(context.Background(), "", "claude", "c-other", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	assert.ErrorIs(t, err, tree.ErrCrossWorkspace)
	assert.Empty(t, chats.Minted)
}

// A BRANCH parent, unlike a CHAT parent, carries no workspace to conflict
// with: a locked branch or repo-home row is a process boundary, not a
// workspace boundary ("Locked means no commits and no branches here, not no
// process here" — 2026-08-23-unified-sidebar-design.md §3.1). A bubble
// placed under one runs in that branch's own worktree regardless of which
// workspace minted it, so the cross-workspace refusal above must NOT fire
// here.
func TestCreateChat_OwnWorktree_AcceptsABranchParentInAnotherWorkspace(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "b-other", Type: domain.ChatTypeBranch, WorkspaceID: "ws-2"})
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), "", "claude", "b-other", tree.WorktreeSpec{Mode: tree.WorktreeFork})
	assert.NoError(t, err)
}

// A create the user was told FAILED must not leave a chat behind — the same
// contract the plain-bubble path's own discard tests pin, now covering the
// failure that is unique to this branch: the slot never got filled at all.
func TestCreateChat_OwnWorktree_TakesTheChatBackOutWhenFillingTheSlotFails(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(chats, "spikes", "")
	chats.NextID = "c-new"
	chats.SpawnOwnWorktreeErr = errors.New("no fork parent")

	_, _, err := uc.CreateChat(context.Background(), "", "claude", "spikes", tree.WorktreeSpec{Mode: tree.WorktreeFork})
	assert.ErrorContains(t, err, "no fork parent")
	assert.Equal(t, []string{"c-new"}, chats.Purged)
}

// A drop onto another chat is the moment a chat BECOMES a thread, and it is the
// only moment anything can record when it started reading that chat. The note
// carries the lineage rather than a bare "you moved", because the record is what
// a reader believes afterwards.
func TestPlaceChat_RecordsTheNewLineageInTheChatsOwnConversation(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		tree.PlaceInput{ParentID: name("c1")})
	require.NoError(t, err)

	assert.Equal(t, []mocks.LineageNote{{ChatID: "c2", Ancestors: []string{"c1"}}}, chats.Noted)
}

// The chain, nearest first — a chat dropped under a thread inherits that
// thread's ancestors too, and the record has to name all of them.
func TestPlaceChat_RecordsTheWholeChainNearestFirst(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)
	seedChat(chats, "c3", 3)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c3",
		tree.PlaceInput{ParentID: name("c2")})
	require.NoError(t, err)

	assert.Equal(t, []mocks.LineageNote{{ChatID: "c3", Ancestors: []string{"c2", "c1"}}}, chats.Noted)
}

// Dropping onto a FOLDER inside a chat makes the chat a thread just the same:
// folders are transparent, so the record names the chat above the folder and
// never the folder.
func TestPlaceChat_AFolderInsideAChatStillRecordsTheChat(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	seedFolder(chats, "outer", "c1")
	seedFolder(chats, "inner", "outer")

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		tree.PlaceInput{ParentID: name("inner")})
	require.NoError(t, err)

	assert.Equal(t, []mocks.LineageNote{{ChatID: "c2", Ancestors: []string{"c1"}}}, chats.Noted)
}

// GAINED, not merely changed. Filing a thread into a folder under the SAME
// parent leaves it reading exactly what it read a moment ago, and announcing a
// context change there would be announcing one that did not happen.
func TestPlaceChat_FilingAThreadUnderItsOwnParentRecordsNothing(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)
	seedFolder(chats, "notes", "c1")

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		tree.PlaceInput{ParentID: name("notes")})
	require.NoError(t, err)

	assert.Empty(t, chats.Noted, "the chat reads the same chat it read before; nothing happened to record")
}

// A chat dragged OUT from under its parent gains nothing. What it no longer
// reads is answered by its next spawn resolving an empty lineage — there is no
// new context to date the start of.
func TestPlaceChat_MovingAThreadBackToTheRootRecordsNothing(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		tree.PlaceInput{ParentID: name("")})
	require.NoError(t, err)

	assert.Empty(t, chats.Noted)
}

// A pure reorder is not a lineage change either.
func TestPlaceChat_AReorderRecordsNothing(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		tree.PlaceInput{Order: index(1)})
	require.NoError(t, err)

	assert.Empty(t, chats.Noted)
}

// The rows have already moved by the time the note is written, so a note that
// fails must not report the move as failed. The relationship rides on ParentID;
// what is lost is the line in the record, not the behaviour it describes.
func TestPlaceChat_AFailedNoteDoesNotFailTheMove(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	chats.NoteErr = errors.New("ledger unwritable")

	placed, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		tree.PlaceInput{ParentID: name("c1")})
	require.NoError(t, err)
	assert.Equal(t, "c1", placed.ParentID)
	assert.Equal(t, "c1", chatRow(t, chats, "c2").ParentID, "and the move stands")
}

// The regression, at the level it is caused: dragging a second chat somewhere
// renumbers the level the FIRST one has just left, and the renumber must not be
// able to put that first chat back.
//
// Two rows dropped into a folder is one gesture in the panel and two placement
// calls to the daemon, sent back to back. The second plans against the read
// model, which can still list the first chat in the level it was dragged out of
// microseconds ago. The densify then hands that chat a fresh index — legitimate,
// it is renumbering the level — and the write that carried the index used to
// carry the snapshot's parent along with it. The user watched a chat they had
// just filed under another chat return to the panel root, and its next session
// came up with no lineage at all.
func TestPlaceChat_ARenumberCannotUndoTheMoveBeforeIt(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "parent", 1)
	seedThread(chats, "moved", "parent", 2)
	seedChat(chats, "second", 3)
	staleAt(chats, "moved", "", 3, 2)

	_, _, err := uc.PlaceChat(ctx, workspaceID, "second",
		tree.PlaceInput{ParentID: name("parent")})
	require.NoError(t, err)

	assert.Equal(t, "parent", chatRow(t, chats, "moved").ParentID,
		"a chat nobody dragged this time must still be where the last drag left it")
	for _, write := range chats.Placed {
		assert.NotEqual(t, "moved", write.ChatID,
			"the row was renumbered, not moved, so nothing may write a parent for it")
	}
}

// The same guarantee stated as the rule rather than the symptom: across a whole
// densify, every row the plan did not re-parent is written through the
// index-only command.
func TestPlaceChat_OnlyTheMovedRowIsWrittenAsAPlacement(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	seedChat(chats, "c3", 3)

	_, _, err := uc.PlaceChat(ctx, workspaceID, "c3",
		tree.PlaceInput{ParentID: name("c1")})
	require.NoError(t, err)

	require.Len(t, chats.Placed, 1)
	assert.Equal(t, mocks.PlacementWrite{ChatID: "c3", ParentID: "c1"}, chats.Placed[0])
	for _, write := range chats.Ordered {
		assert.NotEqual(t, "c3", write.ChatID, "the moved row is not also renumbered separately")
	}
}

// A drag WITHIN one level names no destination, so the level is read off the
// chat itself — and reading it from the projection is how a reorder became a
// move. A chat filed under another chat a moment ago was still listed at the
// root, so "keep it where it is" resolved to the root and dragged it out.
func TestPlaceChat_AReorderKeepsTheParentTheLogHasNotTheProjectedOne(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "parent", 1)
	seedThread(chats, "t1", "parent", 2)
	seedThread(chats, "t2", "parent", 3)
	staleAt(chats, "t1", "", 0, 2)

	placed, _, err := uc.PlaceChat(ctx, workspaceID, "t1", tree.PlaceInput{Order: index(1)})
	require.NoError(t, err)

	assert.Equal(t, "parent", placed.ParentID)
	assert.Equal(t, "parent", chatRow(t, chats, "t1").ParentID,
		"reordering a thread within its parent must not lift it out of that parent")
	assert.Empty(t, chats.Placed, "nothing about any parent was decided here")
}

// A create is a move too: the chat is minted at the root and placed under its
// parent, so it must be written as a placement or the thread is born unthreaded.
func TestCreateChat_TheNewChatIsWrittenAsAPlacement(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone})
	require.NoError(t, err)

	require.Len(t, chats.Placed, 1)
	assert.Equal(t, mocks.PlacementWrite{ChatID: "c-new", ParentID: "c1"}, chats.Placed[0])
}

// A move whose subject the projection has not caught up on still densifies the
// level it is actually leaving, because the origin is read from the log too.
func TestPlaceChat_TheLevelLeftBehindIsTheOneTheLogNames(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "parent", 1)
	seedThread(chats, "t1", "parent", 2)
	seedThread(chats, "t2", "parent", 3)
	staleAt(chats, "t1", "", 0, 2)

	_, _, err := uc.PlaceChat(ctx, workspaceID, "t1", tree.PlaceInput{ParentID: name("")})
	require.NoError(t, err)

	assert.Empty(t, chatRow(t, chats, "t1").ParentID)
	assert.Equal(t, 0, chatRow(t, chats, "t2").Order,
		"the sibling left behind closes up to the front of the level it still shares with nobody")
}

// A chat the projection is behind on is still refused as a container for itself,
// because the cycle guard runs over the plan and the plan holds the subject.
func TestPlaceChat_StillRefusesAChatUnderItself(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	staleAt(chats, "c1", "", 0, 1)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		tree.PlaceInput{ParentID: name("c1")})
	assert.ErrorIs(t, err, tree.ErrCycle)
}

// The subject is resolved through the log fold, so a failure there is the
// failure a caller sees — not a silent fall back to the projected row.
func TestPlaceChat_SurfacesASubjectLoadFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.LoadErr = errNoLog

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		tree.PlaceInput{Order: index(0)})
	assert.ErrorContains(t, err, "log unavailable")
}

// A chat's parent IS its context lineage, so this write legitimately turns a
// standalone chat into a thread of another and back.
func TestPlaceChat_RewritesLineage(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)

	placed, _, err := uc.PlaceChat(ctx, workspaceID, "c2", tree.PlaceInput{ParentID: name("c1")})
	require.NoError(t, err)
	assert.Equal(t, "c1", placed.ParentID)
	assert.Equal(t, 0, placed.Order)
	assert.Equal(t, "c1", chatRow(t, chats, "c2").ParentID)

	back, _, err := uc.PlaceChat(ctx, workspaceID, "c2", tree.PlaceInput{ParentID: name("")})
	require.NoError(t, err)
	assert.Equal(t, "", back.ParentID)
}

// Chats and folders share one level, so a chat drop renumbers the folders in it
// and those rows have to come back for broadcast.
func TestPlaceChat_ReturnsTheFoldersItShifted(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	folder, _, err := uc.Create(ctx, tree.CreateInput{RepoID: repoID, Name: "spikes"})
	require.NoError(t, err)
	require.Equal(t, 1, folder.Order)

	_, shifted, err := uc.PlaceChat(ctx, workspaceID, "c1", tree.PlaceInput{Order: index(1)})
	require.NoError(t, err)
	require.Len(t, shifted, 1)
	assert.Equal(t, folder.ID, shifted[0].ID)
	assert.Equal(t, 0, shifted[0].Order, "the folder took the slot the chat left")
}

func TestPlaceChat_WithNothingRequestedKeepsThePlacement(t *testing.T) {
	chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)

	placed, _, err := uc.PlaceChat(ctx, workspaceID, "c2", tree.PlaceInput{})
	require.NoError(t, err)
	assert.Equal(t, "c1", placed.ParentID)
}

func TestPlaceChat_RefusesAChatOntoItself(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		tree.PlaceInput{ParentID: name("c1")})
	assert.ErrorIs(t, err, tree.ErrCycle)
}

// A chat inside its own subtree is worse than unreachable: its context walk
// would never terminate at the root.
func TestPlaceChat_RefusesAMoveIntoItsOwnThread(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		tree.PlaceInput{ParentID: name("c2")})
	assert.ErrorIs(t, err, tree.ErrCycle)
}

func TestPlaceChat_RefusesAChatFromAnotherWorkspace(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", WorkspaceID: "ws-2"})

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c-other", tree.PlaceInput{})
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestPlaceChat_RefusesAnUnknownChat(t *testing.T) {
	_, uc := newUsecase(t)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "nowhere", tree.PlaceInput{})
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestPlaceChat_SurfacesASnapshotFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.ListErr = errors.New("boom")

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1", tree.PlaceInput{})
	assert.ErrorContains(t, err, "boom")
}

// A reorder inside one level writes indices and no parents, so the write it can
// fail on is the renumber — for the subject as much as for the rows it passed.
func TestPlaceChat_SurfacesARenumberWriteFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	chats.OrderErr = errors.New("aggregate down")

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		tree.PlaceInput{Order: index(0)})
	assert.ErrorContains(t, err, "aggregate down")
}

func TestPlaceChat_SurfacesAPlacementWriteFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	chats.SetErr = errors.New("aggregate down")

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		tree.PlaceInput{ParentID: name("c1")})
	assert.ErrorContains(t, err, "aggregate down")
}

// newUsecaseOverRoster builds the tree usecase over a workspace census the
// reaper actually removes from, so a delete's effect on a WORKSPACE can be
// asserted as absence from that census — not as "a method was called".
//
// The holder census it returns last resolves against the SAME chat rows the
// usecase plans over, through the real worktree.ChatsForWorkspace: seeding a
// sibling that shares a worktree is therefore enough to make the delete see it,
// with nothing to keep in sync by hand.
func newUsecaseOverRoster(
	t *testing.T,
	workspaces ...domain.Workspace,
) (
	*mocks.AgentChatPlacements,
	tree.Usecase,
	*mocks.AgentWorkspaceRoster,
	*mocks.AgentWorkspaceReaper,
	*mocks.AgentWorkspaceHolders,
) {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	roster := mocks.NewAgentWorkspaceRoster()
	roster.Rows = append(roster.Rows, workspaces...)
	reaper := mocks.NewAgentWorkspaceReaperOver(roster)
	holders := mocks.NewAgentWorkspaceHolders(chats)
	uc := tree.New(chats, chats, inflight.NewWork(),
		mocks.NewAgentWorkspaceGitStatus(), roster, reaper, holders)
	return chats, uc, roster, reaper, holders
}

// seedWorktreeChat appends a chat that OWNS wsID — the row a branch or a fork
// is, as distinct from a bubble that borrows its ancestor's ground.
func seedWorktreeChat(
	chats *mocks.AgentChatPlacements,
	id string,
	wsID string,
	parentID string,
) {
	chats.Rows = append(chats.Rows, domain.Chat{
		ID:          id,
		Type:        domain.ChatTypeChat,
		WorkspaceID: wsID,
		ParentID:    parentID,
	})
}

// TestRegression_DeletingAWorkspaceOwningChatAlsoDeletesTheWorkspace is the
// test for the bug spec §0 diagnosed, reached from the other direction.
//
// A workspace is reachable ONLY through the chat that owns it. DeleteChat used
// to purge the chat and its threads and stop there — chat, runner, ledger — so
// deleting the row that owned a worktree left a real git worktree on disk with
// nothing anywhere able to name it again: an orphan identical in shape to the
// one the import path produced, and produced by the ordinary panel delete every
// user makes.
//
// The assertion is deliberately about the CENSUS and not about the reap call:
// "the workspace is gone" is the invariant, and a test that only proved a
// method ran would still pass if that method stopped tearing anything down.
func TestRegression_DeletingAWorkspaceOwningChatAlsoDeletesTheWorkspace(t *testing.T) {
	chats, uc, roster, _, _ := newUsecaseOverRoster(t, domain.Workspace{ID: "ws-fork"})
	seedWorktreeChat(chats, "owner", "ws-fork", "")

	removed, err := uc.DeleteChat(context.Background(), "owner")

	require.NoError(t, err)
	assert.Equal(t, []string{"owner"}, removed.Chats, "the chat row goes")
	assert.Empty(t, roster.Rows,
		"and so does the worktree it owned — a workspace must never outlive its chat")
}

// TestRegression_DeletingAWorkspaceOwningChatReapsItsWorktreeOnce pins the
// cascade over the whole SUBTREE, and the deduplication that keeps it honest.
//
// A thread carries its parent's workspace id, so a chat and the threads under
// it routinely name ONE worktree between them. Every row in the subtree is
// therefore considered, but the teardown runs once per distinct workspace: the
// worktree must go, and it must not be asked to go twice.
func TestRegression_DeletingAWorkspaceOwningChatReapsItsWorktreeOnce(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, domain.Workspace{ID: "ws-root"})
	seedWorktreeChat(chats, "root", "ws-root", "")
	seedWorktreeChat(chats, "thread", "ws-root", "root")

	removed, err := uc.DeleteChat(context.Background(), "root")

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"thread", "root"}, removed.Chats,
		"the chat and its threads all go")
	assert.Empty(t, roster.Rows, "and the worktree they shared goes with them")
	assert.Equal(t, []string{"ws-root"}, reaper.Reaped,
		"torn down once, however many rows named it")
}

// TestRegression_AFailedWorktreeReapLeavesTheChatAndItsWorkspaceIntact pins the
// failure contract the reap ORDER exists to give.
//
// The two orderings fail in opposite directions, and only one of them is safe.
// Reaping first means a failure leaves a chat that still exists and still owns
// its worktree — nothing lost, and the user can retry. Purging first would mean
// a failure leaves the worktree behind with its only route to it deleted, which
// is precisely the orphan this cascade was added to prevent. So a reap that
// fails FAILS THE DELETE, and nothing is purged.
func TestRegression_AFailedWorktreeReapLeavesTheChatAndItsWorkspaceIntact(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, domain.Workspace{ID: "ws-locked"})
	seedWorktreeChat(chats, "owner", "ws-locked", "")
	wedged := errors.New("worktree is locked")
	reaper.ErrFor["ws-locked"] = wedged

	_, err := uc.DeleteChat(context.Background(), "owner")

	require.ErrorIs(t, err, wedged)
	assert.Empty(t, chats.Purged,
		"a delete that cannot tear the worktree down must not erase the only row naming it")
	assert.Len(t, roster.Rows, 1, "and the worktree itself is untouched")
}

// TestDeleteChat_ABubbleReapsNothing keeps the cascade off the ground a bubble
// merely BORROWS. A chat with no workspace of its own reads its ancestor's
// worktree; reaping on its delete would destroy the directory its parent is
// still working in.
func TestDeleteChat_ABubbleReapsNothing(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, domain.Workspace{ID: "ws-parent"})
	seedWorktreeChat(chats, "bubble", "", "")

	_, err := uc.DeleteChat(context.Background(), "bubble")

	require.NoError(t, err)
	assert.Empty(t, reaper.Reaped, "a bubble owns no worktree to tear down")
	assert.Len(t, roster.Rows, 1, "its ancestor's worktree is untouched")
}

// TestRegression_DeletingOneOfTwoChatsSharingAWorktreeSparesIt is the other
// half of the reap contract, and the data-loss bug the first half introduced.
//
// The cascade above reads Chat.WorkspaceID to find what a doomed subtree owns.
// But that field says which worktree a chat is ANCHORED to, never that it is
// anchored there alone: a worktree is many-chats-to-one by design, which is the
// entire premise of the shared reads (git, review, files, search, identity)
// that fan out over exactly this set. Deleting one conversation therefore
// cascade-deleted the worktree its unrelated SIBLINGS were working in and left
// every one of them anchored to a workspace that no longer existed.
//
// The two rows here are siblings, neither inside the other's subtree, so
// nothing about the delete reaches "kept" — and the worktree must survive
// because "kept" is still holding it.
func TestRegression_DeletingOneOfTwoChatsSharingAWorktreeSparesIt(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, domain.Workspace{ID: "ws-shared"})
	seedWorktreeChat(chats, "doomed", "ws-shared", "")
	seedWorktreeChat(chats, "kept", "ws-shared", "")

	removed, err := uc.DeleteChat(context.Background(), "doomed")

	require.NoError(t, err)
	assert.Equal(t, []string{"doomed"}, removed.Chats, "only the named chat goes")
	assert.Empty(t, reaper.Reaped,
		"a worktree a surviving sibling still holds must never be cascaded away")
	assert.Len(t, roster.Rows, 1, "so the workspace is still there")
	assert.Equal(t, "ws-shared", chatRow(t, chats, "kept").WorkspaceID,
		"and the sibling is still anchored to ground that exists")
}

// TestRegression_DeletingTheLastChatHoldingAWorktreeStillReapsIt is the gate
// read in the opposite direction, and it is the test that keeps the fix above
// from quietly undoing the orphan fix it was built on top of.
//
// Skipping the reap while a holder remains is only correct if the reap still
// runs the moment the LAST one goes. Both deletes are made in sequence over one
// census precisely so the gate is observed opening: the same workspace, the
// same reaper, spared once and torn down once, decided entirely by whether
// anything outside the doomed subtree was still pointing at it.
func TestRegression_DeletingTheLastChatHoldingAWorktreeStillReapsIt(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, domain.Workspace{ID: "ws-shared"})
	seedWorktreeChat(chats, "first", "ws-shared", "")
	seedWorktreeChat(chats, "last", "ws-shared", "")
	ctx := context.Background()

	_, err := uc.DeleteChat(ctx, "first")
	require.NoError(t, err)
	require.Empty(t, reaper.Reaped, "one holder still remains")

	_, err = uc.DeleteChat(ctx, "last")

	require.NoError(t, err)
	assert.Equal(t, []string{"ws-shared"}, reaper.Reaped,
		"nothing is left holding it, so the worktree goes")
	assert.Empty(t, roster.Rows, "a workspace no chat can name again must not survive")
}

// TestRegression_AThreadInTheDoomedSubtreeDoesNotCountAsAHolder pins the
// SUBTRACTION the gate is built on.
//
// A thread carries its parent's workspace id, so the doomed subtree's own rows
// appear in the workspace's holder census — and a gate that merely asked "does
// anyone hold this?" would see them, decline every reap, and reintroduce the
// orphan from the other direction. The rows about to be purged are subtracted
// first; only what OUTLIVES the delete can spare a worktree.
func TestRegression_AThreadInTheDoomedSubtreeDoesNotCountAsAHolder(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, domain.Workspace{ID: "ws-root"})
	seedWorktreeChat(chats, "root", "ws-root", "")
	seedWorktreeChat(chats, "thread", "ws-root", "root")

	_, err := uc.DeleteChat(context.Background(), "root")

	require.NoError(t, err)
	assert.Equal(t, []string{"ws-root"}, reaper.Reaped,
		"a chat's own threads are going with it, so they cannot be what keeps its worktree alive")
	assert.Empty(t, roster.Rows)
}

// TestRegression_AFailedHolderCensusRefusesTheDelete pins the gate's failure
// direction, which is the same one the reap itself already has.
//
// The whole reason the census is asked is that "is anything else holding this
// worktree?" cannot be answered from the doomed rows alone. A delete that
// swallowed the error and cascaded anyway would be back to assuming sole
// ownership — the exact assumption that destroyed a shared worktree — so an
// unanswerable question fails the delete, and nothing is purged or torn down.
func TestRegression_AFailedHolderCensusRefusesTheDelete(t *testing.T) {
	chats, uc, roster, reaper, holders := newUsecaseOverRoster(t, domain.Workspace{ID: "ws-fork"})
	seedWorktreeChat(chats, "owner", "ws-fork", "")
	unreadable := errors.New("chat forest unavailable")
	holders.Err = unreadable

	_, err := uc.DeleteChat(context.Background(), "owner")

	require.ErrorIs(t, err, unreadable)
	assert.Empty(t, reaper.Reaped, "a worktree is never torn down on a guess")
	assert.Empty(t, chats.Purged, "and the row naming it stays, so the user can retry")
	assert.Len(t, roster.Rows, 1)
}
