package tree_test

import (
	"context"
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

// rebindWorld is one repo with a default checkout ws-a holding thread c1 and
// its child c2, and a branch b owning ws-b.
func rebindWorld(t *testing.T) (*mocks.AgentChatPlacements, tree.Usecase) {
	t.Helper()
	chats, _, _, gitStatus, uc := newWorkspacePlacementUsecase(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "b", Type: domain.ChatTypeBranch, OwnsWorkspace: true, WorkspaceID: "ws-b"},
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"},
		domain.Chat{ID: "c2", Type: domain.ChatTypeChat, WorkspaceID: "ws-a", ParentID: "c1"},
	)
	gitStatus.SetRepo("ws-a", repoID)
	gitStatus.SetRepo("ws-b", repoID)
	chats.Moves.Finished = make(chan string, 8)
	return chats, uc
}

func waitMoved(t *testing.T, chats *mocks.AgentChatPlacements, ids ...string) {
	t.Helper()
	for range ids {
		select {
		case <-chats.Moves.Finished:
		case <-time.After(5 * time.Second):
			t.Fatalf("moves of %v did not finish", ids)
		}
	}
}

func rowOf(chats *mocks.AgentChatPlacements, id string) domain.Chat {
	for _, r := range chats.Rows {
		if r.ID == id {
			return r
		}
	}
	return domain.Chat{}
}

func TestMoveChat_AThreadDroppedIntoABranchRunsThereWithItsChildren(t *testing.T) {
	chats, uc := rebindWorld(t)
	parent := "b"
	_, _, err := uc.PlaceChat(context.Background(), "ws-a", "c1",
		tree.PlaceInput{ParentID: &parent, WorkspaceID: "ws-b", Label: "b"})
	require.NoError(t, err)
	waitMoved(t, chats, "c1", "c2")

	c1, c2 := rowOf(chats, "c1"), rowOf(chats, "c2")
	assert.Equal(t, "ws-b", c1.WorkspaceID)
	assert.False(t, c1.OwnsWorkspace, "a moved thread never claims the branch it joins")
	assert.Equal(t, "ws-b", c2.WorkspaceID, "its threads follow it")
	done := chats.Moves.Done["c1"]
	require.Len(t, done, 1)
	assert.Equal(t, "/dirs/ws-a", done[0].FromDir)
	assert.Equal(t, "/dirs/ws-b", done[0].ToDir)
}

func TestMoveChat_AWorkingThreadMovesOnlyOnceItsTurnEnds(t *testing.T) {
	chats, uc := rebindWorld(t)
	chats.SetBusy("c1", true)
	parent := "b"
	_, _, err := uc.PlaceChat(context.Background(), "ws-a", "c1",
		tree.PlaceInput{ParentID: &parent, WorkspaceID: "ws-b"})
	require.NoError(t, err)
	assert.Equal(t, "ws-a", rowOf(chats, "c1").WorkspaceID)
	assert.Equal(t, "ws-a", rowOf(chats, "c2").WorkspaceID, "a thread never runs apart from its parent")

	chats.SetBusy("c1", false)
	uc.MoveWhenReady(context.Background(), "c1")
	waitMoved(t, chats, "c1", "c2")
	assert.Equal(t, "ws-b", rowOf(chats, "c1").WorkspaceID)
	assert.Equal(t, "ws-b", rowOf(chats, "c2").WorkspaceID)
}

func TestMoveChat_RefusesADropIntoItsOwnThread(t *testing.T) {
	_, uc := rebindWorld(t)
	parent := "c2"
	_, _, err := uc.PlaceChat(context.Background(), "ws-a", "c1",
		tree.PlaceInput{ParentID: &parent, WorkspaceID: "ws-b"})
	require.ErrorIs(t, err, tree.ErrCycle)
}

func TestMoveChat_ABranchsOwnThreadDoesNotMoveAlone(t *testing.T) {
	chats, uc := rebindWorld(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "own", Type: domain.ChatTypeChat, OwnsWorkspace: true, WorkspaceID: "ws-c"})
	_, _, err := uc.PlaceChat(context.Background(), "ws-c", "own", tree.PlaceInput{WorkspaceID: "ws-b"})
	assert.Equal(t, problem.Conflict, problem.CodeOf(err))
}

func TestResumeMoves_RunsWhatAPreviousDaemonLeftAskedFor(t *testing.T) {
	chats, uc := rebindWorld(t)
	chats.SetBusy("c1", true)
	chats.SetBusy("c2", true)
	parent := "b"
	_, _, err := uc.PlaceChat(context.Background(), "ws-a", "c1",
		tree.PlaceInput{ParentID: &parent, WorkspaceID: "ws-b"})
	require.NoError(t, err)

	chats.SetBusy("c1", false)
	chats.SetBusy("c2", false)
	require.NoError(t, uc.ResumeMoves(context.Background()))
	waitMoved(t, chats, "c1", "c2")
	assert.Equal(t, "ws-b", rowOf(chats, "c1").WorkspaceID)
	assert.Equal(t, "ws-b", rowOf(chats, "c2").WorkspaceID)
}

// A thread's parent can live only in the placement tree (its Node), not on its row:
// it still follows the thread made a branch.
func TestBranchOut_AThreadPlacedOnlyByItsNodeFollowsToo(t *testing.T) {
	chats, _, nodes, gitStatus, uc := newWorkspacePlacementUsecase(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"},
		domain.Chat{ID: "c2", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"},
	)
	nodes.Rows = append(nodes.Rows, dtree.Node{ID: "c2", Kind: dtree.KindChat, ParentID: "c1"})
	gitStatus.SetRepo("ws-a", repoID)
	chats.Moves.Finished = make(chan string, 8)

	require.NoError(t, uc.BranchOut(context.Background(), "c1", ""))
	waitMoved(t, chats, "c1", "c2")
	assert.Equal(t, "ws-cut", rowOf(chats, "c2").WorkspaceID)
}
