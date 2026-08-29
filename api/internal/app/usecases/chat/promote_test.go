package chat_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

var errPromoteBoom = errors.New("promote: boom")

// seedBubbleChat mints a workspace-less chat ("bubble", model spec §3.1),
// threads it under an existing workspace-anchored chat — so it has a real fork
// parent to promote from — and starts a live runner on it so Promote has a
// current provider to respawn. It returns the bubble's id, its runner's id,
// and the workspace-anchored chat it was threaded under.
//
// Placement happens BEFORE StartRunner, mirroring CreateChat's own required
// order (mint, place, start): StartRunner's spawn path now resolves a bubble's
// cwd through tree.CwdWorkspaceID's ancestor walk, which needs rootChatID
// already on the bubble's own ParentID.
func seedBubbleChat(
	t *testing.T,
	f testFixture,
	provider string,
) (bubbleID, runnerID, rootChatID string) {
	t.Helper()
	rootChatID, _ = f.spawn(t, provider)

	bubbleID, err := f.usecase.MintChat(f.ctx, "")
	require.NoError(t, err)
	_, err = f.chats.SetPlacement(f.ctx, bubbleID, rootChatID, 0)
	require.NoError(t, err)
	runnerID, err = f.usecase.StartRunner(f.ctx, bubbleID, provider)
	require.NoError(t, err)
	f.wait()
	return bubbleID, runnerID, rootChatID
}

func TestPromote_FillsWorkspaceKeepsIdentity(t *testing.T) {
	f := newFixture(t)
	bubbleID, oldRunnerID, rootChatID := seedBubbleChat(t, f, "claude")
	before := f.chat(t, bubbleID)
	oldTerm := f.runner(t, oldRunnerID).TerminalSession

	promoted, err := f.usecase.Promote(f.ctx, bubbleID)
	require.NoError(t, err)

	assert.Equal(t, bubbleID, promoted.ID, "promotion must keep the chat's id")
	assert.NotEmpty(t, promoted.WorkspaceID, "promotion must fill WorkspaceID")
	assert.Equal(t, before.Title, promoted.Title, "promotion must keep the chat's title")

	assert.Equal(t, []string{"ws1"}, f.wt.calls(),
		"must fork the NEW workspace from the resolved fork parent, ws1 (rootChatID's own workspace)")
	_ = rootChatID

	// The respawn genuinely went through SwitchProvider: the old CLI was quit and
	// a new one is live on the same chat.
	assert.Contains(t, f.term.terminatedIDs(), oldTerm, "the outgoing CLI is quit gracefully")
	live, err := f.liveRunnerFor(t, bubbleID)
	require.NoError(t, err)
	assert.NotEqual(t, oldRunnerID, live.ID, "a NEW runner is placed for the respawn")
	assert.Equal(t, "claude", live.ProviderID, "respawns the SAME provider the chat was already on")
}

func TestPromote_AlreadyPromoted_Refuses(t *testing.T) {
	f := newFixture(t)
	chatID, err := f.usecase.MintChat(f.ctx, "ws1")
	require.NoError(t, err)

	_, err = f.usecase.Promote(f.ctx, chatID)

	require.ErrorIs(t, err, agentusecase.ErrAlreadyPromoted)
	assert.Empty(t, f.wt.calls(), "must refuse before ever reaching the worktree usecase")
}

func TestPromote_NoForkParent_Refuses(t *testing.T) {
	f := newFixture(t)
	bubbleID, err := f.usecase.MintChat(f.ctx, "")
	require.NoError(t, err)
	f.wait()

	_, err = f.usecase.Promote(f.ctx, bubbleID)

	require.ErrorIs(t, err, agentusecase.ErrNoForkParent)
	assert.Empty(t, f.wt.calls())
}

func TestPromote_NoProviderHistory_Refuses(t *testing.T) {
	f := newFixture(t)
	rootChatID, _ := f.spawn(t, "claude")
	bubbleID, err := f.usecase.MintChat(f.ctx, "")
	require.NoError(t, err)
	_, err = f.chats.SetPlacement(f.ctx, bubbleID, rootChatID, 0)
	require.NoError(t, err)
	f.wait()

	_, err = f.usecase.Promote(f.ctx, bubbleID)

	require.ErrorIs(t, err, agentusecase.ErrNothingToPromote)
	assert.Empty(t, f.wt.calls(), "must refuse before ever reaching the worktree usecase")
}

func TestPromote_CreateWorkspaceFailure_AbortsBeforeSettingTheWorkspace(t *testing.T) {
	f := newFixture(t)
	bubbleID, _, _ := seedBubbleChat(t, f, "claude")
	f.wt.err = errPromoteBoom

	_, err := f.usecase.Promote(f.ctx, bubbleID)

	require.ErrorIs(t, err, errPromoteBoom)
	chat := f.chat(t, bubbleID)
	assert.Empty(t, chat.WorkspaceID, "a failed create must leave the chat exactly as it was")
}

// TestPromote_SetWorkspaceFailure_DiscardsTheOrphanedWorkspace pins the first
// of the two compensations. The workspace is minted BEFORE the row that would
// own it, so a failure between the two leaves a real worktree on disk that no
// row points at — unreachable from the sidebar forever, since the only way to
// reach a workspace is through the row that owns it.
//
// It is taken back out rather than logged, matching tree.Create's discardFolder
// and chats.go's CreateChat→discard: the failure the caller is told about is
// always the one that actually happened, and the half-made thing goes.
func TestPromote_SetWorkspaceFailure_DiscardsTheOrphanedWorkspace(t *testing.T) {
	f, chats, _ := newFaultFixture(t)
	bubbleID, _, _ := seedBubbleChat(t, f, "claude")
	chats.failSetWorkspace = errPromoteBoom

	_, err := f.usecase.Promote(f.ctx, bubbleID)

	require.ErrorIs(t, err, errPromoteBoom, "the caller is told the failure that happened")
	assert.Equal(t, []string{"ws-child-1"}, f.wt.discards(),
		"the workspace nothing came to own must be taken back out")
	assert.Empty(t, f.chat(t, bubbleID).WorkspaceID, "and the chat is the bubble it was")
}

// TestPromote_RespawnFailure_RollsTheWholePromotionBack pins the second. A
// respawn that fails leaves the chat pointing at a brand-new worktree with no
// CLI in it and no way to finish the move: Promote refuses an already-promoted
// chat (ErrAlreadyPromoted), so a retry is impossible and the user is stuck
// with a half-promoted row.
//
// Rolling back — clear the slot, then discard the workspace — puts the world
// exactly where it was, which is discard's own philosophy: the retry the user
// will make is the same call they just made.
func TestPromote_RespawnFailure_RollsTheWholePromotionBack(t *testing.T) {
	f := newFixture(t)
	bubbleID, _, _ := seedBubbleChat(t, f, "claude")
	f.wait()
	f.term.terminateErr = errors.New("boom: the outgoing CLI would not die")

	_, err := f.usecase.Promote(f.ctx, bubbleID)

	require.Error(t, err)
	assert.Empty(t, f.chat(t, bubbleID).WorkspaceID,
		"a chat that could not be respawned is a bubble again, and Promote can be retried")
	assert.Equal(t, []string{"ws-child-1"}, f.wt.discards(),
		"the workspace the failed promotion made must not survive it")
}

// TestPromote_RespawnFailure_KeepsTheWorkspaceWhenTheSlotWillNotClear is the
// ordering that makes the rollback safe rather than destructive: the workspace
// is discarded only AFTER the row has stopped pointing at it. A clear that
// fails must therefore leave the worktree exactly where it is — deleting it
// under a row that still owns it would turn a failed promotion into a chat
// anchored to a worktree that no longer exists.
func TestPromote_RespawnFailure_KeepsTheWorkspaceWhenTheSlotWillNotClear(t *testing.T) {
	f, chats, _ := newFaultFixture(t)
	bubbleID, _, _ := seedBubbleChat(t, f, "claude")
	f.term.terminateErr = errors.New("boom: the outgoing CLI would not die")
	chats.failClearWorkspace = errPromoteBoom

	_, err := f.usecase.Promote(f.ctx, bubbleID)

	require.Error(t, err)
	assert.Empty(t, f.wt.discards(),
		"a workspace a row still points at must never be deleted out from under it")
}

// TestPromote_AppendsThePromotionNote proves the model spec §4.2 ledger note:
// appended AFTER the respawn, into the chat's own conversation, following the
// same "[Crowbar] ..." convention lineageNoteText uses for a lineage change.
//
// This scenario — an actively-chatting bubble whose live runner has already
// announced a session and taken a turn on the provider Promote is about to
// respawn as — also reaches a REAL, KNOWN gap, asserted explicitly below
// rather than triggered silently: model spec §4.2 says "native resume is
// unavailable because a vendor session is cwd-keyed, so promotion always
// takes the... spawned fresh with the whole ledger... branch," but
// SwitchProvider's own resumability check (internal/runner/switch.go,
// resumableConversation) has no notion of workspace identity — it decides
// purely from whether this chat has ever recorded a turn under
// (provider, sessionID). It finds this exact prior session "resumable" and
// hands the incoming CLI a native --resume of it inside the BRAND NEW
// worktree. Fixing that is out of this task's scope (it means changing
// SwitchProvider's own resumability decision, which the brief's hard
// constraint says not to touch) — recorded here instead, mirroring
// TestSwitchProvider_SwitchBack_ResumesOverAPINotTheRedundantPTY's own
// "record the gap, don't assume it's fixed" convention in this same file.
func TestPromote_AppendsThePromotionNote(t *testing.T) {
	f := newFixture(t)
	bubbleID, runnerID, _ := seedBubbleChat(t, f, "claude")
	f.announce(t, runnerID, "sid-bubble")
	turn(t, f, runnerID, "claude", "hello from the bubble")

	_, err := f.usecase.Promote(f.ctx, bubbleID)
	require.NoError(t, err)
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, bubbleID, 0, 0, 0)
	require.NoError(t, err)
	require.NotEmpty(t, page.Items)
	last := page.Items[len(page.Items)-1]
	assert.Contains(t, last.Text, "[Crowbar]")
	assert.Contains(t, last.Text, "promoted")

	// KNOWN GAP (see doc comment above): the respawn resumed the OLD
	// (pre-promotion) native session INSIDE THE NEW WORKTREE instead of
	// spawning fresh with the whole ledger, as the model spec requires.
	argv := f.term.calls[len(f.term.calls)-1].argv
	resumeIdx := indexOf(argv, "--resume")
	require.GreaterOrEqual(t, resumeIdx, 0, "argv %v must contain --resume", argv)
	assert.Equal(t, "sid-bubble", argv[resumeIdx+1],
		"documents the gap: this is the PRE-PROMOTION session id, resumed in the NEW worktree")
}
