package agentchatfolder_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agentchatfolder"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

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
		WorkspaceID: workspaceID,
		ParentID:    parentID,
		Order:       order,
		CreatedAt:   time.Unix(createdAtSec, 0).UTC(),
	}
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
	_, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "parent", 1)
	seedThread(chats, "moved", "parent", 2)
	seedChat(chats, "second", 3)
	staleAt(chats, "moved", "", 3, 2)

	_, _, err := uc.PlaceChat(ctx, workspaceID, "second",
		agentchatfolder.PlaceInput{ParentID: name("parent")})
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
	_, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	seedChat(chats, "c2", 2)
	seedChat(chats, "c3", 3)

	_, _, err := uc.PlaceChat(ctx, workspaceID, "c3",
		agentchatfolder.PlaceInput{ParentID: name("c1")})
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
	_, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "parent", 1)
	seedThread(chats, "t1", "parent", 2)
	seedThread(chats, "t2", "parent", 3)
	staleAt(chats, "t1", "", 0, 2)

	placed, _, err := uc.PlaceChat(ctx, workspaceID, "t1", agentchatfolder.PlaceInput{Order: index(1)})
	require.NoError(t, err)

	assert.Equal(t, "parent", placed.ParentID)
	assert.Equal(t, "parent", chatRow(t, chats, "t1").ParentID,
		"reordering a thread within its parent must not lift it out of that parent")
	assert.Empty(t, chats.Placed, "nothing about any parent was decided here")
}

// Deleting a folder promotes what was inside it, so those rows really did change
// level and their parents really must be written — the fix must not turn every
// chat write into a renumber.
func TestDelete_PromotedChatsAreWrittenAsRealMoves(t *testing.T) {
	folders, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "c1", 1)
	created, _, err := uc.Create(ctx, agentchatfolder.CreateInput{
		WorkspaceID: workspaceID, Name: "spikes",
	})
	require.NoError(t, err)
	_, _, err = uc.PlaceChat(ctx, workspaceID, "c1",
		agentchatfolder.PlaceInput{ParentID: name(created.ID)})
	require.NoError(t, err)
	chats.Placed = nil

	_, err = uc.Delete(ctx, workspaceID, created.ID)
	require.NoError(t, err)

	assert.Equal(t, "", chatRow(t, chats, "c1").ParentID, "the chat came back up to the root")
	require.Len(t, chats.Placed, 1)
	assert.Equal(t, "c1", chats.Placed[0].ChatID)
	require.Empty(t, folderRows(t, folders), "and the folder itself is gone")
}

// A create is a move too: the chat is minted at the root and placed under its
// parent, so it must be written as a placement or the thread is born unthreaded.
func TestCreateChat_TheNewChatIsWrittenAsAPlacement(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.NextID = "c-new"

	_, _, err := uc.CreateChat(context.Background(), workspaceID, "claude", "c1")
	require.NoError(t, err)

	require.Len(t, chats.Placed, 1)
	assert.Equal(t, mocks.PlacementWrite{ChatID: "c-new", ParentID: "c1"}, chats.Placed[0])
}

// A move whose subject the projection has not caught up on still densifies the
// level it is actually leaving, because the origin is read from the log too.
func TestPlaceChat_TheLevelLeftBehindIsTheOneTheLogNames(t *testing.T) {
	_, chats, uc := newUsecase(t)
	ctx := context.Background()
	seedChat(chats, "parent", 1)
	seedThread(chats, "t1", "parent", 2)
	seedThread(chats, "t2", "parent", 3)
	staleAt(chats, "t1", "", 0, 2)

	_, _, err := uc.PlaceChat(ctx, workspaceID, "t1", agentchatfolder.PlaceInput{ParentID: name("")})
	require.NoError(t, err)

	assert.Empty(t, chatRow(t, chats, "t1").ParentID)
	assert.Equal(t, 0, chatRow(t, chats, "t2").Order,
		"the sibling left behind closes up to the front of the level it still shares with nobody")
}

// A chat the projection is behind on is still refused as a container for itself,
// because the cycle guard runs over the plan and the plan holds the subject.
func TestPlaceChat_StillRefusesAChatUnderItself(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	staleAt(chats, "c1", "", 0, 1)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		agentchatfolder.PlaceInput{ParentID: name("c1")})
	assert.ErrorIs(t, err, agentchatfolder.ErrCycle)
}

// The subject is resolved through the log fold, so a failure there is the
// failure a caller sees — not a silent fall back to the projected row.
func TestPlaceChat_SurfacesASubjectLoadFailure(t *testing.T) {
	_, chats, uc := newUsecase(t)
	seedChat(chats, "c1", 1)
	chats.LoadErr = errNoLog

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		agentchatfolder.PlaceInput{Order: index(0)})
	assert.ErrorContains(t, err, "log unavailable")
}
