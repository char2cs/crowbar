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

func TestDeleteChat_RefusesAFolderID(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(t, uc, "spikes", "")
	seedThread(chats, "c1", "spikes", 1)

	_, err := uc.DeleteChat(context.Background(), "spikes", domain.KeepWorkAtRisk)

	require.ErrorIs(t, err, problem.ErrNotFound)
	assert.Empty(t, chats.Purged, "a folder id must never reach the chat cascade")
}

func TestPlaceChat_RefusesAFolderID(t *testing.T) {
	_, uc := newUsecase(t)
	seedFolder(t, uc, "spikes", "")
	seedFolder(t, uc, "ideas", "")

	_, _, err := uc.PlaceChat(context.Background(), "", "spikes", tree.PlaceInput{ParentID: name("ideas")})

	require.ErrorIs(t, err, problem.ErrNotFound)
	assert.Equal(t, "", folderRow(t, uc, "spikes").ParentID, "nothing may have moved")
}

func TestPlaceChat_ABubbleDoesNotSeeEveryFolderTwice(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(t, uc, "f1", "")
	seedFolder(t, uc, "f2", "")
	chats.Rows = append(chats.Rows, domain.Chat{
		ID: "b1", Type: domain.ChatTypeChat, ParentID: "f1",
	})

	_, _, err := uc.PlaceChat(context.Background(), "", "b1", tree.PlaceInput{ParentID: name("")})
	require.NoError(t, err)

	assert.Equal(t, 2, chatRow(t, chats, "b1").Order,
		"the root holds two folders, so a bubble joining it takes slot 2")
}

func TestCreateChat_AtTheRootIsPlacedAtTheNextSlot(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	chats.NextID = "c-new"

	chatID, runnerID, err := createChat(context.Background(), uc, workspaceID, "claude", "", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	assert.Equal(t, "runner-c-new", runnerID)
	assert.Equal(t, []string{workspaceID}, chats.Minted, "minted in its workspace, then placed")
	require.Len(t, chats.Started, 1)
	assert.Equal(t, "", nodeRowFor(t, nodes, "c-new").ParentID)
}

func TestCreateChat_MintsWithTheRequestedLandingSurface(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, workspaceID, "codex", "", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "terminal")

	require.NoError(t, err)
	assert.Equal(t, []string{"terminal"}, chats.MintedSurfaces)
}

func TestCreateChat_NoSurfaceAskedForMintsTheProvidersDefault(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")

	require.NoError(t, err)
	assert.Equal(t, []string{""}, chats.MintedSurfaces)
}

func TestRegression_CreateChat_MintsTheChatWithItsProvider(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, workspaceID, "codex", "", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")

	require.NoError(t, err)
	assert.Equal(t, []string{"codex"}, chats.MintedProviders,
		"the row records its vendor from the instant it exists, not from a later spawn")
}

func TestRegression_CreateChat_OwnWorktreeMintsTheChatWithItsProvider(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"
	fork := tree.WorktreeSpec{Mode: tree.WorktreeFork}
	_, _, err := createChat(context.Background(), uc, workspaceID, "", "", fork, "")
	require.ErrorIs(t, err, tree.ErrForkNeedsProvider)
	assert.Empty(t, chats.Minted)
	_, _, err = createChat(context.Background(), uc, workspaceID, "codex", "", fork, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"codex"}, chats.MintedProviders)
}

func TestCreateChat_PlacesTheChatBeforeStartingItsCLI(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"

	chatID, runnerID, err := createChat(context.Background(), uc, workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
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

func TestCreateChat_InAFolderPlacesItThereToo(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	seedFolder(t, uc, "spikes", "")
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "spikes", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.NoError(t, err)
	assert.Equal(t, "spikes", nodeRowFor(t, nodes, "c-new").ParentID)
}

func TestCreateChat_LandsAtTheEndOfItsParentsSiblingSpace(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.NoError(t, err)
	assert.Equal(t, 1, nodeRowFor(t, nodes, "c-new").Order)
}

func TestCreateChat_DoesNotReorderUnrelatedRootSiblings(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "bystander", 2)
	nodes.Rows = append(nodes.Rows,
		dtree.Node{ID: "c1", Kind: dtree.KindChat, ParentID: "", Order: 0},
		dtree.Node{ID: "bystander", Kind: dtree.KindChat, ParentID: "", Order: 5},
	)
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.NoError(t, err)

	assert.Equal(t, 5, nodeRowFor(t, nodes, "bystander").Order,
		"an unrelated root sibling's order must survive a thread create on c1 untouched")
	for _, w := range nodes.Ordered {
		assert.NotEqual(t, "bystander", w.ID,
			"threading off c1 rewrote an unrelated root sibling's order via SetOrder")
	}
	for _, w := range nodes.Placed {
		assert.NotEqual(t, "bystander", w.ID,
			"threading off c1 rewrote an unrelated root sibling's placement via SetPlacement")
	}
}

func TestCreateChat_StillGoesThroughThePlacementPath(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.NoError(t, err)
	assert.NotEmpty(t, nodes.Rows, "the placement is written through Node, like any other repo-scoped row")
}

func TestCreateChat_RefusesAnUnknownParentWithoutMintingAnything(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "nowhere", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.Error(t, err)
	assert.Empty(t, chats.Minted)
	assert.Empty(t, chats.Started)
}

func TestCreateChat_AcceptsAFolderParentRegardlessOfProvenance(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(t, uc, "f-other", "")
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "f-other", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	assert.NoError(t, err)
}

func TestCreateChat_RefusesAChatParentInAnotherWorkspace(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", WorkspaceID: "ws-2"})

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "c-other", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	assert.ErrorIs(t, err, tree.ErrCrossWorkspace)
	assert.Empty(t, chats.Minted)
}

func TestCreateChat_SurfacesASnapshotFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.ListErr = errors.New("folders down")

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	assert.ErrorContains(t, err, "folders down")
	assert.Empty(t, chats.Minted)
}

func TestCreateChat_SurfacesAMintFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.MintErr = errors.New("mint down")

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	assert.ErrorContains(t, err, "mint down")
	assert.Empty(t, chats.Purged, "nothing was minted, so there is nothing to take back")
}

func TestCreateChat_TakesTheChatBackOutWhenThePlacementFails(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"
	nodes.CreateErr = errors.New("placement down")

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	assert.ErrorContains(t, err, "placement down")
	assert.Equal(t, []string{"c-new"}, chats.Purged)
	assert.Empty(t, chats.Started, "and no CLI is started on a chat that is about to be erased")
}

func TestCreateChat_TakesTheChatBackOutWhenTheCLIFailsToStart(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"
	chats.StartErr = errors.New("claude is not installed")

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	assert.ErrorContains(t, err, "claude is not installed")
	assert.Equal(t, []string{"c-new"}, chats.Purged)
}

func TestCreateChat_AFailedCleanupStillReportsTheOriginalFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"
	chats.StartErr = errors.New("claude is not installed")
	chats.PurgeErr = errors.New("purge down")

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	assert.ErrorContains(t, err, "claude is not installed")
	assert.NotErrorIs(t, err, problem.ErrNotFound)
}

func TestCreateChat_OwnWorktree_AtTheRootSkipsPlacementBeforeFillingTheSlot(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	chatID, runnerID, err := createChat(context.Background(), uc, "some-other-ws", "claude", "", tree.WorktreeSpec{Mode: tree.WorktreeFork}, "")
	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	assert.Equal(t, "runner-c-new", runnerID)
	assert.Equal(t, []string{""}, chats.Minted, "the row is minted as a plain bubble, not into the caller's workspaceID")
	assert.Empty(t, chats.Placed, "there is nowhere to place a root-level chat")
	require.Len(t, chats.SpawnedOwnWorktree, 1)
	assert.Equal(t, "c-new", chats.SpawnedOwnWorktree[0].ChatID)
	assert.Equal(t, "claude", chats.SpawnedOwnWorktree[0].ProviderID)
}

func TestCreateChat_OwnWorktree_PlacesTheChatBeforeFillingItsSlot(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(t, uc, "spikes", "")
	chats.NextID = "c-new"

	chatID, runnerID, err := createChat(context.Background(), uc, "some-other-ws", "claude", "spikes", tree.WorktreeSpec{Mode: tree.WorktreeFork}, "")
	require.NoError(t, err)
	assert.Equal(t, "c-new", chatID)
	assert.Equal(t, "runner-c-new", runnerID)
	assert.Equal(t, "spikes", chatRow(t, chats, "c-new").ParentID)
	require.Len(t, chats.SpawnedOwnWorktree, 1)
	assert.Equal(t, "spikes", chats.SpawnedOwnWorktree[0].ParentAtStart,
		"the fork-parent walk must see the placement, or it resolves nothing")
}

func TestCreateChat_OwnWorktree_InAFolderPlacesItThereToo(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(t, uc, "spikes", "")
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, "", "claude", "spikes", tree.WorktreeSpec{Mode: tree.WorktreeFork}, "")
	require.NoError(t, err)
	assert.Equal(t, "spikes", chatRow(t, chats, "c-new").ParentID)
}

func TestCreateChat_OwnWorktree_RefusesAnUnknownParentWithoutMintingAnything(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, "", "claude", "nowhere", tree.WorktreeSpec{Mode: tree.WorktreeFork}, "")
	require.Error(t, err)
	assert.Empty(t, chats.Minted)
}

func TestCreateChat_OwnWorktree_AcceptsAWorktreeOwningChatParentInAnotherWorkspace(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", Type: domain.ChatTypeChat, WorkspaceID: "ws-2"})
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, "", "claude", "c-other", tree.WorktreeSpec{Mode: tree.WorktreeFork}, "")
	assert.NoError(t, err)
	assert.Equal(t, "c-other", chatRow(t, chats, "c-new").ParentID)
}

func TestCreateChat_RefusesAChatParentInAnotherWorkspaceEvenWithNoWorkspaceOfItsOwn(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", Type: domain.ChatTypeChat, WorkspaceID: "ws-2"})

	_, _, err := createChat(context.Background(), uc, "", "claude", "c-other", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.ErrorIs(t, err, tree.ErrThreadNeedsWorkspace)
	assert.Equal(t, problem.InvalidArgument, problem.Of(err).Code, "a missing field, not a conflict")
	assert.Empty(t, chats.Minted)
}

func TestCreateChat_OwnWorktree_AcceptsABranchParentInAnotherWorkspace(t *testing.T) {
	chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "b-other", Type: domain.ChatTypeBranch, WorkspaceID: "ws-2"})
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, "", "claude", "b-other", tree.WorktreeSpec{Mode: tree.WorktreeFork}, "")
	assert.NoError(t, err)
}

func TestCreateChat_OwnWorktree_TakesTheChatBackOutWhenFillingTheSlotFails(t *testing.T) {
	chats, uc := newUsecase(t)
	seedFolder(t, uc, "spikes", "")
	chats.NextID = "c-new"
	chats.SpawnOwnWorktreeErr = errors.New("no fork parent")

	_, _, err := createChat(context.Background(), uc, "", "claude", "spikes", tree.WorktreeSpec{Mode: tree.WorktreeFork}, "")
	assert.ErrorContains(t, err, "no fork parent")
	assert.Equal(t, []string{"c-new"}, chats.Purged)
}

func TestPlaceChat_RecordsTheNewLineageInTheChatsOwnConversation(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		tree.PlaceInput{ParentID: name("c1")})
	require.NoError(t, err)

	assert.Equal(t, []mocks.LineageNote{{ChatID: "c2", Ancestors: []string{"c1"}}}, chats.Noted)
}

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

func TestPlaceChat_AFolderInsideAChatStillRecordsTheChat(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	seedFolder(t, uc, "outer", "c1")
	seedFolder(t, uc, "inner", "outer")

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		tree.PlaceInput{ParentID: name("inner")})
	require.NoError(t, err)

	assert.Equal(t, []mocks.LineageNote{{ChatID: "c2", Ancestors: []string{"c1"}}}, chats.Noted)
}

func TestPlaceChat_FilingAThreadUnderItsOwnParentRecordsNothing(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)
	seedFolder(t, uc, "notes", "c1")

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		tree.PlaceInput{ParentID: name("notes")})
	require.NoError(t, err)

	assert.Empty(t, chats.Noted, "the chat reads the same chat it read before; nothing happened to record")
}

func TestPlaceChat_MovingAThreadBackToTheRootRecordsNothing(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		tree.PlaceInput{ParentID: name("")})
	require.NoError(t, err)

	assert.Empty(t, chats.Noted)
}

func TestPlaceChat_AReorderRecordsNothing(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		tree.PlaceInput{Order: index(1)})
	require.NoError(t, err)

	assert.Empty(t, chats.Noted)
}

func TestPlaceChat_AFailedNoteDoesNotFailTheMove(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	chats.NoteErr = errors.New("ledger unwritable")

	placed, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		tree.PlaceInput{ParentID: name("c1")})
	require.NoError(t, err)
	assert.Equal(t, "c1", placed.ParentID)
	assert.Equal(t, "c1", nodeRowFor(t, nodes, "c2").ParentID, "and the move stands")
}

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

func TestPlaceChat_OnlyTheMovedRowIsWrittenAsAPlacement(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	seedChat(chats, "c3", 3)

	_, _, err := uc.PlaceChat(ctx, workspaceID, "c3",
		tree.PlaceInput{ParentID: name("c1")})
	require.NoError(t, err)

	assert.Equal(t, "c1", nodeRowFor(t, nodes, "c3").ParentID)
	assert.Empty(t, nodes.Placed, "c3 is a fresh Node mint (Create), not a SetPlacement")
	for _, write := range nodes.Ordered {
		assert.NotEqual(t, "c3", write.ID, "the moved row is not also renumbered separately")
	}
}

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

func TestCreateChat_TheNewChatIsWrittenAsAPlacement(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"

	_, _, err := createChat(context.Background(), uc, workspaceID, "claude", "c1", tree.WorktreeSpec{Mode: tree.WorktreeNone}, "")
	require.NoError(t, err)

	assert.Equal(t, "c1", nodeRowFor(t, nodes, "c-new").ParentID)
}

func TestPlaceChat_TheLevelLeftBehindIsTheOneTheLogNames(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	ctx := context.Background()
	seedChat(chats, "parent", 1)
	seedThread(chats, "t1", "parent", 2)
	seedThread(chats, "t2", "parent", 3)
	staleAt(chats, "t1", "", 0, 2)

	_, _, err := uc.PlaceChat(ctx, workspaceID, "t1", tree.PlaceInput{ParentID: name("")})
	require.NoError(t, err)

	assert.Empty(t, nodeRowFor(t, nodes, "t1").ParentID)
	assert.Equal(t, 0, chatRow(t, chats, "t2").Order,
		"the sibling left behind closes up to the front of the level it still shares with nobody")
}

func TestPlaceChat_StillRefusesAChatUnderItself(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	staleAt(chats, "c1", "", 0, 1)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		tree.PlaceInput{ParentID: name("c1")})
	assert.ErrorIs(t, err, tree.ErrCycle)
}

func TestPlaceChat_SurfacesASubjectLoadFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.LoadErr = errNoLog

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		tree.PlaceInput{Order: index(0)})
	assert.ErrorContains(t, err, "log unavailable")
}

func TestPlaceChat_RewritesLineage(t *testing.T) {
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)

	placed, _, err := uc.PlaceChat(ctx, workspaceID, "c2", tree.PlaceInput{ParentID: name("c1")})
	require.NoError(t, err)
	assert.Equal(t, "c1", placed.ParentID)
	assert.Equal(t, 0, placed.Order)
	assert.Equal(t, "c1", nodeRowFor(t, nodes, "c2").ParentID)

	back, _, err := uc.PlaceChat(ctx, workspaceID, "c2", tree.PlaceInput{ParentID: name("")})
	require.NoError(t, err)
	assert.Equal(t, "", back.ParentID)
}

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
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestPlaceChat_RefusesAnUnknownChat(t *testing.T) {
	_, uc := newUsecase(t)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "nowhere", tree.PlaceInput{})
	assert.ErrorIs(t, err, problem.ErrNotFound)
}

func TestPlaceChat_SurfacesASnapshotFailure(t *testing.T) {
	chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.ListErr = errors.New("boom")

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1", tree.PlaceInput{})
	assert.ErrorContains(t, err, "boom")
}

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
	chats, _, nodes, uc, _ := newUsecaseWithStores(t)
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	nodes.CreateErr = errors.New("aggregate down")

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c2",
		tree.PlaceInput{ParentID: name("c1")})
	assert.ErrorContains(t, err, "aggregate down")
}

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
	gitStatus := mocks.NewAgentWorkspaceGitStatus()
	for _, ws := range workspaces {
		gitStatus.SetRepo(ws.ID, ws.RepoID)
	}
	uc := tree.New(chats, chats, newBusyChats(),
		gitStatus, reaper, holders,
		mocks.NewFolderStore(), mocks.NewNodePlacements())
	return chats, uc, roster, reaper, holders
}

func worktreeIn(id string) domain.Workspace {
	return domain.Workspace{ID: id, RepoID: repoID}
}

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

func TestRegression_DeletingAWorkspaceOwningChatAlsoDeletesTheWorkspace(t *testing.T) {
	chats, uc, roster, _, _ := newUsecaseOverRoster(t, worktreeIn("ws-fork"))
	seedWorktreeChat(chats, "owner", "ws-fork", "")

	removed, err := uc.DeleteChat(context.Background(), "owner", domain.KeepWorkAtRisk)

	require.NoError(t, err)
	assert.Equal(t, []string{"owner"}, removed.Chats, "the chat row goes")
	assert.Empty(t, roster.Rows,
		"and so does the worktree it owned — a workspace must never outlive its chat")
}

func TestRegression_DeletingAWorkspaceOwningChatReapsItsWorktreeOnce(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, worktreeIn("ws-root"))
	seedWorktreeChat(chats, "root", "ws-root", "")
	seedWorktreeChat(chats, "thread", "ws-root", "root")

	removed, err := uc.DeleteChat(context.Background(), "root", domain.KeepWorkAtRisk)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"thread", "root"}, removed.Chats,
		"the chat and its threads all go")
	assert.Empty(t, roster.Rows, "and the worktree they shared goes with them")
	assert.Equal(t, []string{"ws-root"}, reaper.Reaped,
		"torn down once, however many rows named it")
}

func TestRegression_AFailedWorktreeReapLeavesTheChatAndItsWorkspaceIntact(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, worktreeIn("ws-locked"))
	seedWorktreeChat(chats, "owner", "ws-locked", "")
	wedged := errors.New("worktree is locked")
	reaper.ErrFor["ws-locked"] = wedged

	_, err := uc.DeleteChat(context.Background(), "owner", domain.KeepWorkAtRisk)

	require.ErrorIs(t, err, wedged)
	assert.Empty(t, chats.Purged,
		"a delete that cannot tear the worktree down must not erase the only row naming it")
	assert.Len(t, roster.Rows, 1, "and the worktree itself is untouched")
}

func TestRegression_DeletingAHomesOnlyChatKeepsTheHome(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, domain.Workspace{ID: "ws-home", Kind: domain.WorkspaceKindHome})
	seedWorktreeChat(chats, "only", "ws-home", "")

	removed, err := uc.DeleteChat(context.Background(), "only", domain.KeepWorkAtRisk)

	require.NoError(t, err)
	assert.Equal(t, []string{"only"}, removed.Chats)
	assert.Empty(t, reaper.Reaped, "a project's home is never reaped by a chat delete")
	assert.Len(t, roster.Rows, 1)
}

func TestDeleteChat_ABubbleReapsNothing(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, worktreeIn("ws-parent"))
	seedWorktreeChat(chats, "bubble", "", "")

	_, err := uc.DeleteChat(context.Background(), "bubble", domain.KeepWorkAtRisk)

	require.NoError(t, err)
	assert.Empty(t, reaper.Reaped, "a bubble owns no worktree to tear down")
	assert.Len(t, roster.Rows, 1, "its ancestor's worktree is untouched")
}

func TestRegression_DeletingOneOfTwoChatsSharingAWorktreeSparesIt(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, worktreeIn("ws-shared"))
	seedWorktreeChat(chats, "doomed", "ws-shared", "")
	seedWorktreeChat(chats, "kept", "ws-shared", "")

	removed, err := uc.DeleteChat(context.Background(), "doomed", domain.KeepWorkAtRisk)

	require.NoError(t, err)
	assert.Equal(t, []string{"doomed"}, removed.Chats, "only the named chat goes")
	assert.Empty(t, reaper.Reaped,
		"a worktree a surviving sibling still holds must never be cascaded away")
	assert.Len(t, roster.Rows, 1, "so the workspace is still there")
	assert.Equal(t, "ws-shared", chatRow(t, chats, "kept").WorkspaceID,
		"and the sibling is still anchored to ground that exists")
}

func TestRegression_DeletingTheLastChatHoldingAWorktreeStillReapsIt(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, worktreeIn("ws-shared"))
	seedWorktreeChat(chats, "first", "ws-shared", "")
	seedWorktreeChat(chats, "last", "ws-shared", "")
	ctx := context.Background()

	_, err := uc.DeleteChat(ctx, "first", domain.KeepWorkAtRisk)
	require.NoError(t, err)
	require.Empty(t, reaper.Reaped, "one holder still remains")

	_, err = uc.DeleteChat(ctx, "last", domain.KeepWorkAtRisk)

	require.NoError(t, err)
	assert.Equal(t, []string{"ws-shared"}, reaper.Reaped,
		"nothing is left holding it, so the worktree goes")
	assert.Empty(t, roster.Rows, "a workspace no chat can name again must not survive")
}

func TestRegression_AThreadInTheDoomedSubtreeDoesNotCountAsAHolder(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t, worktreeIn("ws-root"))
	seedWorktreeChat(chats, "root", "ws-root", "")
	seedWorktreeChat(chats, "thread", "ws-root", "root")

	_, err := uc.DeleteChat(context.Background(), "root", domain.KeepWorkAtRisk)

	require.NoError(t, err)
	assert.Equal(t, []string{"ws-root"}, reaper.Reaped,
		"a chat's own threads are going with it, so they cannot be what keeps its worktree alive")
	assert.Empty(t, roster.Rows)
}

func TestRegression_AFailedHolderCensusRefusesTheDelete(t *testing.T) {
	chats, uc, roster, reaper, holders := newUsecaseOverRoster(t, worktreeIn("ws-fork"))
	seedWorktreeChat(chats, "owner", "ws-fork", "")
	unreadable := errors.New("chat forest unavailable")
	holders.Err = unreadable

	_, err := uc.DeleteChat(context.Background(), "owner", domain.KeepWorkAtRisk)

	require.ErrorIs(t, err, unreadable)
	assert.Empty(t, reaper.Reaped, "a worktree is never torn down on a guess")
	assert.Empty(t, chats.Purged, "and the row naming it stays, so the user can retry")
	assert.Len(t, roster.Rows, 1)
}

func TestDeleteChat_WithoutConsentRefusesOverWorkAtRiskBeforeReapingAnything(t *testing.T) {
	chats, uc, roster, reaper, _ := newUsecaseOverRoster(t,
		worktreeIn("ws-root"), worktreeIn("ws-child"))
	seedWorktreeChat(chats, "root", "ws-root", "")
	seedWorktreeChat(chats, "child", "ws-child", "root")
	risk := domain.WorkAtRisk{WorkspaceID: "ws-root", Branch: "feature/root", UncommittedFiles: 2}
	reaper.Risks = map[string]domain.WorkAtRisk{"ws-root": risk}

	_, err := uc.DeleteChat(context.Background(), "root", domain.KeepWorkAtRisk)

	require.ErrorIs(t, err, domain.ErrWorkAtRisk)
	assert.Equal(t, map[string]any{"workAtRisk": []domain.WorkAtRisk{risk}}, problem.DataOf(err))
	assert.Empty(t, reaper.Reaped, "not even the clean child is reaped")
	assert.Empty(t, chats.Purged)
	assert.Len(t, roster.Rows, 2)

	_, err = uc.DeleteChat(context.Background(), "root", domain.DiscardWorkAtRisk)

	require.NoError(t, err)
	assert.Empty(t, roster.Rows)
	assert.Equal(t, []domain.DeleteConsent{domain.DiscardWorkAtRisk, domain.DiscardWorkAtRisk}, reaper.Consents)
}
