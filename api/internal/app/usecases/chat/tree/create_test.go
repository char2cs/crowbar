package tree_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// A chat with nowhere in particular to go takes the unplaced spawn, untouched.
// There is no edge to write, so there is no gap to open between minting and
// starting, and a plain new chat must be created in exactly the order it always
// was.
func TestCreateChat_AtTheRootTakesTheUnplacedSpawn(t *testing.T) {
	_, chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	chatID, runnerID, err := uc.CreateChat(context.Background(), workspaceID, "claude", "")
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
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"

	chatID, runnerID, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1")
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
	folders, chats, uc := newUsecase(t)
	seedFolder(folders, "spikes", "")
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "spikes")
	require.NoError(t, err)
	assert.Equal(t, "spikes", chatRow(t, chats, "c-new").ParentID)
}

// A new chat lands at the END of its parent's sibling space, the same rule a new
// folder follows — the placement usecase's own, not a second copy of it.
func TestCreateChat_LandsAtTheEndOfItsParentsSiblingSpace(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1")
	require.NoError(t, err)
	assert.Equal(t, 1, chatRow(t, chats, "c-new").Order)
}

// A chat BORN under a parent gets no "this chat was moved" note: it was not
// moved, and it has nothing above the line for such a note to date. The note is
// suppressed by the ledger being empty, which is the agent usecase's call — here
// we only prove the create routes through the same PlaceChat every drag does.
func TestCreateChat_StillGoesThroughThePlacementPath(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1")
	require.NoError(t, err)
	assert.Positive(t, chats.SetCalls, "the placement is written through the chat aggregate, like any other")
}

// A parent that does not exist is refused before anything is minted or spawned.
// Minting first and cleaning up afterwards would make a create and a delete out
// of every mistyped id.
func TestCreateChat_RefusesAnUnknownParentWithoutMintingAnything(t *testing.T) {
	_, chats, uc := newUsecase(t)
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "nowhere")
	require.Error(t, err)
	assert.Empty(t, chats.Minted)
	assert.Empty(t, chats.Started)
	assert.Empty(t, chats.Spawned)
}

// A row in ANOTHER workspace is the cross-workspace refusal, not a not-found: a
// chat parent is what the row READS, so accepting it would let a new agent
// inherit context from a workspace the user is not in.
func TestCreateChat_RefusesAParentInAnotherWorkspace(t *testing.T) {
	folders, chats, uc := newUsecase(t)
	folders.Rows = append(folders.Rows, domain.ChatFolder{ID: "f-other", WorkspaceID: "ws-2"})

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "f-other")
	assert.ErrorIs(t, err, tree.ErrCrossWorkspace)
	assert.Empty(t, chats.Minted)
}

// A chat parent in another workspace is refused the same way.
func TestCreateChat_RefusesAChatParentInAnotherWorkspace(t *testing.T) {
	_, chats, uc := newUsecase(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c-other", WorkspaceID: "ws-2"})

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c-other")
	assert.ErrorIs(t, err, tree.ErrCrossWorkspace)
	assert.Empty(t, chats.Minted)
}

func TestCreateChat_SurfacesASnapshotFailure(t *testing.T) {
	folders, chats, uc := newUsecase(t)
	folders.FindErr = errors.New("folders down")

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1")
	assert.ErrorContains(t, err, "folders down")
	assert.Empty(t, chats.Minted)
}

func TestCreateChat_SurfacesAMintFailure(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.MintErr = errors.New("mint down")

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1")
	assert.ErrorContains(t, err, "mint down")
	assert.Empty(t, chats.Purged, "nothing was minted, so there is nothing to take back")
}

// A create the user was told FAILED must not leave a chat behind. Everything past
// the mint therefore takes the chat back out again.
func TestCreateChat_TakesTheChatBackOutWhenThePlacementFails(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"
	chats.SetErr = errors.New("placement down")

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1")
	assert.ErrorContains(t, err, "placement down")
	assert.Equal(t, []string{"c-new"}, chats.Purged)
	assert.Empty(t, chats.Started, "and no CLI is started on a chat that is about to be erased")
}

func TestCreateChat_TakesTheChatBackOutWhenTheCLIFailsToStart(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"
	chats.StartErr = errors.New("claude is not installed")

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1")
	assert.ErrorContains(t, err, "claude is not installed")
	assert.Equal(t, []string{"c-new"}, chats.Purged)
}

// The purge is best-effort and never replaces the cause: the user asked to create
// a chat, and THAT is what failed. Reporting the cleanup's error instead would
// name a failure they cannot act on and hide the one they can.
func TestCreateChat_AFailedCleanupStillReportsTheOriginalFailure(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"
	chats.StartErr = errors.New("claude is not installed")
	chats.PurgeErr = errors.New("purge down")

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1")
	assert.ErrorContains(t, err, "claude is not installed")
	assert.NotErrorIs(t, err, apperr.ErrNotFound)
}
