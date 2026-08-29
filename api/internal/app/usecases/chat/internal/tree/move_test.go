package tree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
)

// newUsecaseWithWork is newUsecase with the in-flight tracker exposed, for the
// tests below that need to mark a row working.
func newUsecaseWithWork(
	t *testing.T,
) (*mocks.AgentChatPlacements, tree.Usecase, *inflight.Work) {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	work := inflight.NewWork()
	return chats, tree.New(chats, chats, work), work
}

// seedFolderTree creates "root" and "other" as sibling folders and files
// child-1 and child-2 as chats under "root", the fixture both Move tests below
// share.
func seedFolderTree(
	t *testing.T,
	chats *mocks.AgentChatPlacements,
	uc tree.Usecase,
) {
	t.Helper()
	ctx := context.Background()
	_, _, err := uc.Create(ctx, tree.CreateInput{ID: "root", RepoID: repoID, Name: "root"})
	require.NoError(t, err)
	_, _, err = uc.Create(ctx, tree.CreateInput{ID: "other", RepoID: repoID, Name: "other"})
	require.NoError(t, err)
	seedThread(chats, "child-1", "root", 1)
	seedThread(chats, "child-2", "root", 2)
}

// A folder move takes its whole subtree — a chat filed under it is part of
// what moves — so a working descendant refuses the move exactly as a working
// chat refuses its own.
func TestMove_RefusesWorkingSubtree(t *testing.T) {
	chats, uc, work := newUsecaseWithWork(t)
	seedFolderTree(t, chats, uc)
	work.Set("child-1", true)

	_, _, err := uc.Move(context.Background(), "root", tree.MoveInput{ParentID: name("other")})
	assert.ErrorIs(t, err, tree.ErrSubtreeWorking)
}

// The subtree travels without a single one of its rows being rewritten: a
// child's ParentID already names the row that moved, so re-parenting "root"
// carries "child-1" and "child-2" with it for free.
func TestMove_TakesWholeSubtree(t *testing.T) {
	chats, uc, _ := newUsecaseWithWork(t)
	seedFolderTree(t, chats, uc)

	placed, _, err := uc.Move(context.Background(), "root", tree.MoveInput{ParentID: name("other")})
	require.NoError(t, err)

	assert.Equal(t, "other", placed.ParentID)
	assert.Equal(t, "root", chatRow(t, chats, "child-1").ParentID)
	assert.Equal(t, "root", chatRow(t, chats, "child-2").ParentID)
}

// PlaceChat makes the identical refusal for a CHAT's own move: a thread below
// the one being dragged is part of the subtree it takes, so it blocks the move
// just as a working chat blocks its own.
func TestPlaceChat_RefusesWorkingSubtree(t *testing.T) {
	chats, uc, work := newUsecaseWithWork(t)
	seedChat(chats, "c1", 1)
	seedThread(chats, "c2", "c1", 2)
	seedChat(chats, "other", 3)
	work.Set("c2", true)

	_, _, err := uc.PlaceChat(context.Background(), workspaceID, "c1",
		tree.PlaceInput{ParentID: name("other")})
	assert.ErrorIs(t, err, tree.ErrSubtreeWorking)
}
