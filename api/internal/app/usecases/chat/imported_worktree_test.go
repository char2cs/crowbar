package chat_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var errImportedWorktreeBoom = errors.New("imported worktree: boom")

func importedBranch() agentusecase.ImportSpec {
	return agentusecase.ImportSpec{
		RepoID:    "repo-1",
		ProjectID: "prj-1",
		RepoPath:  "/repo",
		Branch:    "feature/pricing-rounding",
	}
}

func TestSpawnChatWithImportedWorktree_FillsWorkspaceAndLeavesTheChatDormant(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	forks := f.term.callCount()

	ws, runnerID, err := f.usecase.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "claude", importedBranch())
	require.NoError(t, err)
	require.NotEmpty(t, ws.ID)

	assert.Equal(t, []string{"feature/pricing-rounding"}, f.wt.imports(),
		"an import must ask the worktree port for the branch it found, not for a fork")
	assert.Empty(t, f.wt.calls(),
		"and must never take the FORK path, which would invent a new branch instead of adopting this one")

	after := f.chat(t, bubbleID)
	assert.Equal(t, ws.ID, after.WorkspaceID, "the slot must be filled with the imported workspace")

	assert.Empty(t, runnerID)
	assert.Equal(t, forks, f.term.callCount(), "no CLI is launched before the chat's first prompt")
}

func TestSpawnChatWithImportedWorktree_OnTheTerminalStartsTheCLI(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubbleOn(t, f, rootChatID, "claude", "terminal")

	_, runnerID, err := f.usecase.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "claude", importedBranch())
	require.NoError(t, err)
	require.NotEmpty(t, runnerID)

	live, err := f.liveRunnerFor(t, bubbleID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, live.ID)
}

func TestSpawnChatWithImportedWorktree_NoProviderStartsNoRunner(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)

	ws, runnerID, err := f.usecase.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "", importedBranch())

	require.NoError(t, err)
	assert.Empty(t, runnerID, "no provider was asked for, so no CLI may be started")
	assert.NotEmpty(t, ws.ID, "the workspace is still created and still owned")
	assert.Equal(t, ws.ID, f.chat(t, bubbleID).WorkspaceID)

	_, err = f.liveRunnerFor(t, bubbleID)
	require.Error(t, err, "there must be no live runner on a row nobody opened")
}

func TestSpawnChatWithImportedWorktree_CreateFailure_AbortsBeforeSettingTheWorkspace(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	f.wt.importErr = errImportedWorktreeBoom

	_, _, err := f.usecase.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "claude", importedBranch())

	require.ErrorIs(t, err, errImportedWorktreeBoom)
	assert.Empty(t, f.chat(t, bubbleID).WorkspaceID,
		"a failed import must leave the chat exactly as it was")
	assert.Empty(t, f.wt.discards(),
		"nothing was created, so there is nothing to discard")
}

func TestSpawnChatWithImportedWorktree_SetWorkspaceFailure_DiscardsTheOrphan(t *testing.T) {
	f, chats := newFaultFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	chats.failSetWorkspace = errImportedWorktreeBoom

	_, _, err := f.usecase.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "claude", importedBranch())

	require.ErrorIs(t, err, errImportedWorktreeBoom, "the caller is told the failure that happened")
	assert.Equal(t, []string{"ws-child-1"}, f.wt.discards(),
		"the workspace nothing came to own must be taken back out")
	assert.Empty(t, f.chat(t, bubbleID).WorkspaceID)
}

func TestSpawnChatWithImportedWorktree_LaunchFailure_RollsTheWholeImportBack(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubbleOn(t, f, rootChatID, "claude", "terminal")
	f.term.err = errImportedWorktreeBoom

	_, _, err := f.usecase.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "claude", importedBranch())

	require.Error(t, err)
	assert.Empty(t, f.chat(t, bubbleID).WorkspaceID,
		"a chat whose CLI never started is a bubble again, and the import can be retried")
	assert.Equal(t, []string{"ws-child-1"}, f.wt.discards(),
		"the workspace a failed import made must not survive it")
}

func TestSpawnChatWithImportedWorktree_StartAndClearFailure_StillDiscardsTheWorkspace(t *testing.T) {
	f, chats := newFaultFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubbleOn(t, f, rootChatID, "claude", "terminal")
	f.term.err = errImportedWorktreeBoom
	chats.failClearWorkspace = errors.New("imported worktree: the slot-clear also failed")

	_, _, err := f.usecase.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "claude", importedBranch())

	require.Error(t, err)
	assert.Equal(t, []string{"ws-child-1"}, f.wt.discards(),
		"the workspace must be discarded even when the slot-clear itself fails — "+
			"nothing survives this path to still claim it")
}

func TestAttachWorkspace_PointsTheRowAtItsWorkspace(t *testing.T) {
	f := newFixture(t)
	chatID, err := f.usecase.MintChat(f.ctx, "", "", "", "")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.own.AttachWorkspace(f.ctx, chatID, "ws-built-elsewhere"))
	f.wait()

	assert.Equal(t, "ws-built-elsewhere", f.chat(t, chatID).WorkspaceID)
}

func TestAttachWorkspace_Failure_IsReported(t *testing.T) {
	f, chats := newFaultFixture(t)
	chatID, err := f.usecase.MintChat(f.ctx, "", "", "", "")
	require.NoError(t, err)
	f.wait()
	chats.failSetWorkspace = errImportedWorktreeBoom

	err = f.usecase.own.AttachWorkspace(f.ctx, chatID, "ws-built-elsewhere")

	require.ErrorIs(t, err, errImportedWorktreeBoom)
	assert.Empty(t, f.chat(t, chatID).WorkspaceID)
}

func TestSpawnChatWithImportedWorktree_ReturnsTheWorkspaceItCreated(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	f.wt.importedWS = domain.Workspace{ID: "ws-locked", Status: domain.WorkspaceStatusLocked}

	ws, _, err := f.usecase.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "", importedBranch())

	require.NoError(t, err)
	assert.Equal(t, "ws-locked", ws.ID)
	assert.Equal(t, domain.WorkspaceStatusLocked, ws.Status,
		"the caller decides the row kind from this, so the lock state has to survive the call")
}
