package chat_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

var errOwnWorktreeBoom = errors.New("own worktree: boom")

// seedPlacedBubble mints a workspace-less chat and threads it under parentID —
// via a direct repository write, exactly like promote_test.go's own
// seedBubbleChat, so it has a real fork parent to resolve — WITHOUT starting a
// runner on it. That is the state CreateChat's createOwnWorktreeChat itself
// produces right before calling SpawnChatWithOwnWorktree (chat/internal/tree/
// chats.go): minted, placed, no CLI yet.
func seedPlacedBubble(
	t *testing.T,
	f testFixture,
	parentID string,
) string {
	t.Helper()
	bubbleID, err := f.usecase.MintChat(f.ctx, "")
	require.NoError(t, err)
	_, err = f.chats.SetPlacement(f.ctx, bubbleID, parentID, 0)
	require.NoError(t, err)
	f.wait()
	return bubbleID
}

// TestSpawnChatWithOwnWorktree_FillsWorkspaceAndStartsTheCLI mirrors
// TestPromote_FillsWorkspaceKeepsIdentity, minus the identity-preservation
// assertions that only make sense for an EXISTING chat: this bubble never had
// a runner, so there is no "old" CLI to compare against — just a fresh one.
func TestSpawnChatWithOwnWorktree_FillsWorkspaceAndStartsTheCLI(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)

	runnerID, err := f.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, "claude")
	require.NoError(t, err)
	require.NotEmpty(t, runnerID)

	assert.Equal(t, []string{"ws1"}, f.wt.calls(),
		"must fork the NEW workspace from the resolved fork parent, ws1 (rootChatID's own workspace)")

	after := f.chat(t, bubbleID)
	assert.NotEmpty(t, after.WorkspaceID, "the slot must be filled")

	live, err := f.liveRunnerFor(t, bubbleID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, live.ID)
	assert.Equal(t, "claude", live.ProviderID, "starts the REQUESTED provider — there is no prior one to preserve")
}

// A chat with no ancestor carrying a workspace has nothing to fork from, the
// same refusal Promote makes for the identical shape (TestPromote_NoForkParent_Refuses).
func TestSpawnChatWithOwnWorktree_NoForkParent_Refuses(t *testing.T) {
	f := newFixture(t)
	bubbleID, err := f.usecase.MintChat(f.ctx, "")
	require.NoError(t, err)
	f.wait()

	_, err = f.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, "claude")

	require.ErrorIs(t, err, agentusecase.ErrNoForkParent)
	assert.Empty(t, f.wt.calls(), "must refuse before ever reaching the worktree usecase")
}

// A workspace-create failure must abort before the chat is ever told about a
// workspace — mirroring TestPromote_CreateWorkspaceFailure_AbortsBeforeSettingTheWorkspace.
func TestSpawnChatWithOwnWorktree_CreateWorkspaceFailure_AbortsBeforeSettingTheWorkspace(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	f.wt.err = errOwnWorktreeBoom

	_, err := f.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, "claude")

	require.ErrorIs(t, err, errOwnWorktreeBoom)
	chat := f.chat(t, bubbleID)
	assert.Empty(t, chat.WorkspaceID, "a failed create must leave the chat exactly as it was")
}

// The workspace is minted BEFORE the row that would own it, so a failure
// setting it leaves a real worktree on disk nothing points at — taken back out
// rather than logged, mirroring TestPromote_SetWorkspaceFailure_DiscardsTheOrphanedWorkspace.
func TestSpawnChatWithOwnWorktree_SetWorkspaceFailure_DiscardsTheOrphanedWorkspace(t *testing.T) {
	f, chats, _ := newFaultFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	chats.failSetWorkspace = errOwnWorktreeBoom

	_, err := f.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, "claude")

	require.ErrorIs(t, err, errOwnWorktreeBoom, "the caller is told the failure that happened")
	assert.Equal(t, []string{"ws-child-1"}, f.wt.discards(),
		"the workspace nothing came to own must be taken back out")
	assert.Empty(t, f.chat(t, bubbleID).WorkspaceID)
}

// A CLI that fails to start leaves the chat pointing at a brand-new worktree
// with nothing running in it — rolled all the way back (clear the slot, then
// discard the workspace) so the retry the caller makes is the same call they
// just made. Mirrors TestPromote_RespawnFailure_RollsTheWholePromotionBack,
// with the CLI failure forced at the create site (f.term.err) rather than at
// the outgoing-CLI teardown Promote's respawn has and this path does not.
func TestSpawnChatWithOwnWorktree_StartRunnerFailure_RollsTheWholeCreateBack(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	f.term.err = errOwnWorktreeBoom

	_, err := f.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, "claude")

	require.Error(t, err)
	assert.Empty(t, f.chat(t, bubbleID).WorkspaceID,
		"a chat whose CLI never started is a bubble again, and the create can be retried")
	assert.Equal(t, []string{"ws-child-1"}, f.wt.discards(),
		"the workspace a failed create made must not survive it")
}

// TestSpawnChatWithOwnWorktree_StartRunnerFailureAndClearFailure_StillDiscardsTheWorkspace
// is the fix for the double fault a review caught: on a StartRunner failure,
// Promote's own unpromote (promote.go) deliberately KEEPS the workspace when
// the slot-clear write itself also fails, because Promote's chat SURVIVES a
// failed promotion and that surviving row still names it. This path has no
// surviving row — CreateChat's ownWorktree branch (tree/chats.go) purges
// chatID outright on ANY error this method returns, clear-failure included —
// so "leave it, a row still points at it" is false here even when the clear
// fails, and the old code (reusing unpromote verbatim) orphaned the workspace
// exactly this way: nothing could ever reach it again, since the only door to
// a workspace is the row that owns it, and that row was about to be gone
// either way.
func TestSpawnChatWithOwnWorktree_StartRunnerFailureAndClearFailure_StillDiscardsTheWorkspace(t *testing.T) {
	f, chats, _ := newFaultFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	f.term.err = errOwnWorktreeBoom
	chats.failClearWorkspace = errors.New("own worktree: the slot-clear also failed")

	_, err := f.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, "claude")

	require.Error(t, err)
	assert.Equal(t, []string{"ws-child-1"}, f.wt.discards(),
		"the workspace must be discarded even when the slot-clear itself fails — "+
			"nothing survives this path to still claim it")
}
