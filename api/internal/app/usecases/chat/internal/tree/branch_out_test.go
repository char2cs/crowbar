package tree_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func TestBranchOut_AThreadBecomesTheBranchCutFromItsFolderAndItsThreadsFollow(t *testing.T) {
	chats, uc := rebindWorld(t)
	require.NoError(t, uc.BranchOut(context.Background(), "c1", "fix-it"))
	waitMoved(t, chats, "c1", "c2")

	c2 := rowOf(chats, "c2")
	assert.Equal(t, []string{"ws-a"}, chats.Moves.BranchOuts, "cut from the folder the thread ran in")
	assert.Equal(t, "ws-cut", rowOf(chats, "c1").WorkspaceID, "the thread owns and runs in its new branch")
	assert.Equal(t, "ws-cut", c2.WorkspaceID, "a thread under it runs in the new branch too")
	assert.Equal(t, "c1", c2.ParentID)
	done := chats.Moves.Done["c1"]
	require.Len(t, done, 1)
	assert.Equal(t, "fix-it", done[0].Branch)
	assert.Equal(t, "abc1234", done[0].Base)
	assert.Equal(t, "/dirs/ws-a", done[0].FromDir)
}

func TestBranchOut_AFailedCutLeavesTheThreadWhereItWas(t *testing.T) {
	chats, uc := rebindWorld(t)
	chats.Moves.BranchOutErr = errors.New("worktree add: boom")
	require.NoError(t, uc.BranchOut(context.Background(), "c1", ""))
	waitMoved(t, chats, "c1")
	assert.Equal(t, "ws-a", rowOf(chats, "c1").WorkspaceID)
	assert.Empty(t, chats.Moves.Done["c1"])
}

func TestBranchOut_RefusesAHomeThread(t *testing.T) {
	chats, uc := rebindWorld(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "h1", Type: domain.ChatTypeChat, WorkspaceID: "home"})
	err := uc.BranchOut(context.Background(), "h1", "")
	require.ErrorIs(t, err, tree.ErrNoRepoToBranch)
}

// The agent's branch-out runs only on the person's Allow; a Deny withdraws it with their reason.
func TestAnswerMove_AllowRunsTheAgentsBranchOutDenyWithdrawsIt(t *testing.T) {
	chats, uc := rebindWorld(t)
	asked := ledger.Move{RequestID: "m1", BranchOut: true, From: "ws-a", By: "agent", AwaitsConsent: true}
	_, err := chats.RequestMove(context.Background(), "c1", asked)
	require.NoError(t, err)
	uc.MoveWhenReady(context.Background(), "c1")
	assert.Equal(t, "ws-a", rowOf(chats, "c1").WorkspaceID, "nothing runs before the person answers")

	require.NoError(t, uc.AnswerMove(context.Background(), "c1", "m1", true, ""))
	waitMoved(t, chats, "c1", "c2")
	assert.Equal(t, "ws-cut", rowOf(chats, "c1").WorkspaceID)

	_, err = chats.RequestMove(context.Background(), "c2",
		ledger.Move{RequestID: "m2", BranchOut: true, From: "ws-cut", By: "agent", AwaitsConsent: true})
	require.NoError(t, err)
	require.NoError(t, uc.AnswerMove(context.Background(), "c2", "m2", false, "not now"))
	assert.Equal(t, "Denied: not now", chats.Moves.Cancelled["c2"])
}
