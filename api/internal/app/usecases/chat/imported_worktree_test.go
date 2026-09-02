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

// importedBranch is the spec a batch importer hands over: a branch that already
// exists, named outright, with its git-lineage parent already resolved.
func importedBranch() agentusecase.ImportSpec {
	return agentusecase.ImportSpec{
		RepoID:    "repo-1",
		ProjectID: "prj-1",
		RepoPath:  "/repo",
		Branch:    "feature/pricing-rounding",
	}
}

// TestSpawnChatWithImportedWorktree_FillsWorkspaceAndStartsTheCLI is the import
// mirror of TestSpawnChatWithOwnWorktree_FillsWorkspaceAndStartsTheCLI, and the
// one assertion that differs is the one that matters: the worktree port is
// asked for a BRANCH, not for a fork parent to branch from.
func TestSpawnChatWithImportedWorktree_FillsWorkspaceAndStartsTheCLI(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)

	ws, runnerID, err := f.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "claude", importedBranch())
	require.NoError(t, err)
	require.NotEmpty(t, runnerID)
	require.NotEmpty(t, ws.ID)

	assert.Equal(t, []string{"feature/pricing-rounding"}, f.wt.imports(),
		"an import must ask the worktree port for the branch it found, not for a fork")
	assert.Empty(t, f.wt.calls(),
		"and must never take the FORK path, which would invent a new branch instead of adopting this one")

	after := f.chat(t, bubbleID)
	assert.Equal(t, ws.ID, after.WorkspaceID, "the slot must be filled with the imported workspace")

	live, err := f.liveRunnerFor(t, bubbleID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, live.ID)
}

// TestSpawnChatWithImportedWorktree_NoProviderStartsNoRunner is the whole
// reason repo-add and batch import can use this path at all.
//
// Materialising a repository's protected branches, or twenty branches off a
// PR chain, creates twenty ROWS — not twenty conversations somebody opened. If
// this started a CLI per row, adding one repo would launch a vendor process for
// every branch in it, on a path the user never asked to talk to.
func TestSpawnChatWithImportedWorktree_NoProviderStartsNoRunner(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)

	ws, runnerID, err := f.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "", importedBranch())

	require.NoError(t, err)
	assert.Empty(t, runnerID, "no provider was asked for, so no CLI may be started")
	assert.NotEmpty(t, ws.ID, "the workspace is still created and still owned")
	assert.Equal(t, ws.ID, f.chat(t, bubbleID).WorkspaceID)

	_, err = f.liveRunnerFor(t, bubbleID)
	require.Error(t, err, "there must be no live runner on a row nobody opened")
}

// A workspace-create failure must abort before the chat is ever told about a
// workspace — the import mirror of the fork path's own first rollback step.
func TestSpawnChatWithImportedWorktree_CreateFailure_AbortsBeforeSettingTheWorkspace(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	f.wt.importErr = errImportedWorktreeBoom

	_, _, err := f.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "claude", importedBranch())

	require.ErrorIs(t, err, errImportedWorktreeBoom)
	assert.Empty(t, f.chat(t, bubbleID).WorkspaceID,
		"a failed import must leave the chat exactly as it was")
	assert.Empty(t, f.wt.discards(),
		"nothing was created, so there is nothing to discard")
}

// The workspace is created BEFORE the row that would own it, so a failure
// setting the slot leaves a real worktree nothing points at — taken back out
// rather than logged.
func TestSpawnChatWithImportedWorktree_SetWorkspaceFailure_DiscardsTheOrphan(t *testing.T) {
	f, chats, _ := newFaultFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	chats.failSetWorkspace = errImportedWorktreeBoom

	_, _, err := f.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "claude", importedBranch())

	require.ErrorIs(t, err, errImportedWorktreeBoom, "the caller is told the failure that happened")
	assert.Equal(t, []string{"ws-child-1"}, f.wt.discards(),
		"the workspace nothing came to own must be taken back out")
	assert.Empty(t, f.chat(t, bubbleID).WorkspaceID)
}

// A CLI that fails to start leaves the chat pointing at a freshly imported
// worktree with nothing running in it — rolled all the way back, so the retry
// the caller makes is the same call they just made.
func TestSpawnChatWithImportedWorktree_StartRunnerFailure_RollsTheWholeImportBack(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	f.term.err = errImportedWorktreeBoom

	_, _, err := f.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "claude", importedBranch())

	require.Error(t, err)
	assert.Empty(t, f.chat(t, bubbleID).WorkspaceID,
		"a chat whose CLI never started is a bubble again, and the import can be retried")
	assert.Equal(t, []string{"ws-child-1"}, f.wt.discards(),
		"the workspace a failed import made must not survive it")
}

// The double fault: StartRunner fails AND the slot-clear that precedes the
// discard fails too. The workspace must STILL be discarded, for the reason
// discardMintedWorkspace's own doc gives — the caller purges this chat outright
// on any error from here, so "leave it, a row still points at it" is false.
func TestSpawnChatWithImportedWorktree_StartAndClearFailure_StillDiscardsTheWorkspace(t *testing.T) {
	f, chats, _ := newFaultFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	f.term.err = errImportedWorktreeBoom
	chats.failClearWorkspace = errors.New("imported worktree: the slot-clear also failed")

	_, _, err := f.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "claude", importedBranch())

	require.Error(t, err)
	assert.Equal(t, []string{"ws-child-1"}, f.wt.discards(),
		"the workspace must be discarded even when the slot-clear itself fails — "+
			"nothing survives this path to still claim it")
}

// TestAttachWorkspace_PointsTheRowAtItsWorkspace covers the bare slot write the
// creation paths that build their own workspace use (repo import's adopt,
// protected-branch provisioning, the placeholder rows).
func TestAttachWorkspace_PointsTheRowAtItsWorkspace(t *testing.T) {
	f := newFixture(t)
	chatID, err := f.usecase.MintChat(f.ctx, "")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.own.AttachWorkspace(f.ctx, chatID, "ws-built-elsewhere"))
	f.wait()

	assert.Equal(t, "ws-built-elsewhere", f.chat(t, chatID).WorkspaceID)
}

// A failed attach is reported rather than swallowed: the caller has a workspace
// row on its hands that nothing owns yet, and only an error tells it to roll
// that row back.
func TestAttachWorkspace_Failure_IsReported(t *testing.T) {
	f, chats, _ := newFaultFixture(t)
	chatID, err := f.usecase.MintChat(f.ctx, "")
	require.NoError(t, err)
	f.wait()
	chats.failSetWorkspace = errImportedWorktreeBoom

	err = f.own.AttachWorkspace(f.ctx, chatID, "ws-built-elsewhere")

	require.ErrorIs(t, err, errImportedWorktreeBoom)
	assert.Empty(t, f.chat(t, chatID).WorkspaceID)
}

// A locked import — which is what every protected branch produces — must come
// back carrying the workspace, because the row's KIND is decided from it. This
// pins the return value the tree's retype depends on rather than the retype
// itself (that is tree's own test).
func TestSpawnChatWithImportedWorktree_ReturnsTheWorkspaceItCreated(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID := seedPlacedBubble(t, f, rootChatID)
	f.wt.importedWS = domain.Workspace{ID: "ws-locked", Status: domain.WorkspaceStatusLocked}

	ws, _, err := f.own.SpawnChatWithImportedWorktree(
		f.ctx, bubbleID, "", importedBranch())

	require.NoError(t, err)
	assert.Equal(t, "ws-locked", ws.ID)
	assert.Equal(t, domain.WorkspaceStatusLocked, ws.Status,
		"the caller decides the row kind from this, so the lock state has to survive the call")
}
