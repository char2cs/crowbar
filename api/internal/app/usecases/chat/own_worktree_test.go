package chat_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
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
	return seedPlacedBubbleOn(t, f, parentID, "", "")
}

// seedPlacedBubbleOn is seedPlacedBubble minted as CreateChat mints it, for
// providerID on surface: a terminal one is the create that launches its CLI.
func seedPlacedBubbleOn(t *testing.T, f testFixture, parentID, providerID, surface string) string {
	t.Helper()
	bubbleID, err := f.usecase.MintChat(f.ctx, "", "", providerID, surface)
	require.NoError(t, err)
	_, err = f.chats.SetPlacement(f.ctx, bubbleID, parentID, 0)
	require.NoError(t, err)
	f.wait()
	return bubbleID
}

// TestSpawnChatWithOwnWorktree_FillsWorkspaceAndLeavesTheChatDormant mirrors
// TestPromote_FillsWorkspaceKeepsIdentity for a chat that never had a runner: on
// the chat surface none is started, its first prompt launches the one CLI.
func TestSpawnChatWithOwnWorktree_FillsWorkspaceAndLeavesTheChatDormant(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	forks := f.term.callCount()

	wsID, runnerID, err := f.usecase.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, "claude", "")
	require.NoError(t, err)

	assert.Equal(t, []string{"ws1"}, f.wt.calls(),
		"must fork the NEW workspace from the resolved fork parent, ws1 (rootChatID's own workspace)")

	after := f.chat(t, bubbleID)
	assert.NotEmpty(t, after.WorkspaceID, "the slot must be filled")
	assert.Equal(t, after.WorkspaceID, wsID, "the workspace it answers is the one it filled the slot with")

	assert.Empty(t, runnerID)
	assert.Equal(t, forks, f.term.callCount(), "no CLI is launched before the chat's first prompt")
	_, err = f.liveRunnerFor(t, bubbleID)
	require.Error(t, err, "the new chat is dormant")
}

// A chat created on the terminal has nothing to show without its CLI, so the
// create launches it, on the REQUESTED provider.
func TestSpawnChatWithOwnWorktree_OnTheTerminalStartsTheCLI(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubbleOn(t, f, rootChatID, "claude", "terminal")

	_, runnerID, err := f.usecase.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, "claude", "")
	require.NoError(t, err)
	require.NotEmpty(t, runnerID)

	live, err := f.liveRunnerFor(t, bubbleID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, live.ID)
	assert.Equal(t, "claude", live.ProviderID, "starts the REQUESTED provider — there is no prior one to preserve")
}

// TestRegression_SpawnChatWithOwnWorktree_ForwardsTheRequestedBranchName pins
// the sidebar's "type the branch name before it forks" flow (2026-09-09): the
// caller-supplied name must reach CreateChildWorkspace verbatim, not be
// silently dropped in favour of the server-generated one every caller before
// this got.
func TestRegression_SpawnChatWithOwnWorktree_ForwardsTheRequestedBranchName(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)

	_, _, err := f.usecase.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, "claude", "feature/typed-name")
	require.NoError(t, err)

	assert.Equal(t, []string{"feature/typed-name"}, f.wt.forkedBranches,
		"the typed branch name must reach the worktree creator, not the auto-generated fallback")
}

// A chat with no ancestor carrying a workspace has nothing to fork from, the
// same refusal Promote makes for the identical shape (TestPromote_NoForkParent_Refuses).
func TestSpawnChatWithOwnWorktree_NoForkParent_Refuses(t *testing.T) {
	f := newFixture(t)
	bubbleID, err := f.usecase.MintChat(f.ctx, "", "", "", "")
	require.NoError(t, err)
	f.wait()

	_, _, err = f.usecase.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, "claude", "")

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

	_, _, err := f.usecase.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, "claude", "")

	require.ErrorIs(t, err, errOwnWorktreeBoom)
	chat := f.chat(t, bubbleID)
	assert.Empty(t, chat.WorkspaceID, "a failed create must leave the chat exactly as it was")
}

// The workspace is minted BEFORE the row that would own it, so a failure
// setting it leaves a real worktree on disk nothing points at — taken back out
// rather than logged, mirroring TestPromote_SetWorkspaceFailure_DiscardsTheOrphanedWorkspace.
func TestSpawnChatWithOwnWorktree_SetWorkspaceFailure_DiscardsTheOrphanedWorkspace(t *testing.T) {
	f, chats := newFaultFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	chats.failSetWorkspace = errOwnWorktreeBoom

	_, _, err := f.usecase.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, "claude", "")

	require.ErrorIs(t, err, errOwnWorktreeBoom, "the caller is told the failure that happened")
	assert.Equal(t, []string{"ws-child-1"}, f.wt.discards(),
		"the workspace nothing came to own must be taken back out")
	assert.Empty(t, f.chat(t, bubbleID).WorkspaceID)
}

// A CLI that fails to start leaves the chat pointing at a brand-new worktree
// with nothing running in it — rolled all the way back (clear the slot, then
// discard the workspace) so the retry the caller makes is the same call they
// just made (a terminal chat: the one a create launches). Mirrors TestPromote_RespawnFailure_RollsTheWholePromotionBack,
// with the CLI failure forced at the create site (f.term.err) rather than at
// the outgoing-CLI teardown Promote's respawn has and this path does not.
func TestSpawnChatWithOwnWorktree_LaunchFailure_RollsTheWholeCreateBack(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubbleOn(t, f, rootChatID, "claude", "terminal")
	f.term.err = errOwnWorktreeBoom

	_, _, err := f.usecase.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, "claude", "")

	require.Error(t, err)
	assert.Empty(t, f.chat(t, bubbleID).WorkspaceID,
		"a chat whose CLI never started is a bubble again, and the create can be retried")
	assert.Equal(t, []string{"ws-child-1"}, f.wt.discards(),
		"the workspace a failed create made must not survive it")
}

// TestSpawnChatWithOwnWorktree_LaunchFailureAndClearFailure_StillDiscardsTheWorkspace
// is the fix for the double fault a review caught: on a launch failure,
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
func TestSpawnChatWithOwnWorktree_LaunchFailureAndClearFailure_StillDiscardsTheWorkspace(t *testing.T) {
	f, chats := newFaultFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubbleOn(t, f, rootChatID, "claude", "terminal")
	f.term.err = errOwnWorktreeBoom
	chats.failClearWorkspace = errors.New("own worktree: the slot-clear also failed")

	_, _, err := f.usecase.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, "claude", "")

	require.Error(t, err)
	assert.Equal(t, []string{"ws-child-1"}, f.wt.discards(),
		"the workspace must be discarded even when the slot-clear itself fails — "+
			"nothing survives this path to still claim it")
}

// createDormantChat is CreateChat's fork path for providerID on the chat
// surface, returning the chat and the forks made before it.
func createDormantChat(t *testing.T, f testFixture, providerID string) (string, int) {
	t.Helper()
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubbleOn(t, f, rootChatID, providerID, "")
	forks := f.term.callCount()
	_, runnerID, err := f.usecase.own.SpawnChatWithOwnWorktree(f.ctx, bubbleID, providerID, "")
	require.NoError(t, err)
	require.Empty(t, runnerID, "a chat-surface create launches nothing")
	f.wait()
	return bubbleID, forks
}

// The first prompt of a new chat launches its CLI exactly once, carrying the
// prompt: the create no longer forks an idle one for the prompt to displace.
func TestRegression_FirstPromptLaunchesCLIOnce(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			f := newFixture(t)
			chatID, forks := createDormantChat(t, f, provider)
			require.Equal(t, forks, f.term.callCount(), "the new chat is dormant until used")

			submitted, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hello there", uuid.NewString(), "", nil)
			require.NoError(t, err)

			require.Equal(t, forks+1, f.term.callCount(), "create plus first prompt is one launch")
			assert.Contains(t, f.term.calls[forks].argv, "hello there", "that launch carries the prompt")
			live, err := f.liveRunnerFor(t, chatID)
			require.NoError(t, err)
			assert.Equal(t, submitted.RunnerID, live.ID)
			_, exited := f.lastExit(t, chatID)
			assert.False(t, exited, "no runner was displaced")
		})
	}
}

// Opening the terminal of a chat nothing ever ran on launches its CLI there,
// once; a second open is the same view.
func TestRegression_TerminalOpenedBeforeAnyPromptLaunchesThere(t *testing.T) {
	f := newFixture(t)
	chatID, forks := createDormantChat(t, f, "claude")

	termSessID, err := f.usecase.SwitchToTerminal(f.ctx, chatID)
	require.NoError(t, err)
	require.NotEmpty(t, termSessID)
	again, err := f.usecase.SwitchToTerminal(f.ctx, chatID)
	require.NoError(t, err)

	assert.Equal(t, termSessID, again)
	assert.Equal(t, forks+1, f.term.callCount())
	assert.Equal(t, "terminal", f.chat(t, chatID).Surface)
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, termSessID, live.TerminalSession)
}

// A dormant chat that already ran only records the move: its CLI comes back
// with a send or an explicit start, as before.
func TestSwitchToTerminal_ADormantChatThatRanOnlyRecordsTheMove(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	require.False(t, f.isLive(runnerID))
	forks := f.term.callCount()

	termSessID, err := f.usecase.SwitchToTerminal(f.ctx, chatID)
	require.NoError(t, err)

	assert.Empty(t, termSessID)
	assert.Equal(t, forks, f.term.callCount())
	assert.Equal(t, "terminal", f.chat(t, chatID).Surface)
}
