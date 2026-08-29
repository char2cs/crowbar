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
func seedBubbleChat(
	t *testing.T,
	f testFixture,
	provider string,
) (bubbleID, runnerID, rootChatID string) {
	t.Helper()
	rootChatID, _ = f.spawn(t, provider)

	bubbleID, err := f.usecase.MintChat(f.ctx, "")
	require.NoError(t, err)
	runnerID, err = f.usecase.StartRunner(f.ctx, bubbleID, provider)
	require.NoError(t, err)
	_, err = f.chats.SetPlacement(f.ctx, bubbleID, rootChatID, 0)
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

// TestPromote_AppendsThePromotionNote proves the model spec §4.2 ledger note:
// appended AFTER the respawn, into the chat's own conversation, following the
// same "[Crowbar] ..." convention lineageNoteText uses for a lineage change.
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
}
