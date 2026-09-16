package runner

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newTestRunnersWithPrompts is a minimal Runners fixture for exercising the
// prompt-journal helpers directly, without the full chat usecase harness.
func newTestRunnersWithPrompts(home string) *Runners {
	return &Runners{
		home:    func() (string, error) { return home, nil },
		prompts: agentjournal.NewPromptRequests(),
	}
}

// TestPromptJournalDirFor_StableAcrossPromotion pins spec §1.5's invariant: a
// chat's ledger location must be a pure function of its own id, never of its
// WorkspaceID. WorkspaceID is optional and mutable (a bubble chat has none
// until promoted), so a chat with no workspace and the SAME chat once promoted
// into one must resolve to the identical journal directory.
//
// Before this fix, the directory came from rs.ws.AgentChatsDir(ctx,
// chat.WorkspaceID), which resolves a workspace row by id — an empty
// WorkspaceID has no such row, so the lookup errored outright rather than
// merely disagreeing with the promoted path. promptJournalDirFor now takes no
// workspace input at all, so nothing about a chat's WorkspaceID can move it.
func TestPromptJournalDirFor_StableAcrossPromotion(t *testing.T) {
	rs := newTestRunnersWithPrompts(t.TempDir())

	before := domain.Chat{ID: "chat-1", WorkspaceID: ""}
	after := domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}

	beforeDir, err := rs.promptJournalDirFor(before.ID)
	require.NoError(t, err, "a workspace-less chat must resolve a journal dir, not error")
	afterDir, err := rs.promptJournalDirFor(after.ID)
	require.NoError(t, err)

	assert.Equal(t, beforeDir, afterDir,
		"ledger path must be a function of chat id, not workspace")
}

// TestPromptJournalDirFor_KeyedByChatID guards the other half of the same
// invariant: two DIFFERENT chats must never collide on one journal directory.
func TestPromptJournalDirFor_KeyedByChatID(t *testing.T) {
	home := t.TempDir()
	rs := newTestRunnersWithPrompts(home)

	dirA, err := rs.promptJournalDirFor("chat-a")
	require.NoError(t, err)
	dirB, err := rs.promptJournalDirFor("chat-b")
	require.NoError(t, err)

	assert.NotEqual(t, dirA, dirB)
	assert.Equal(t, filepath.Join(home, "chats", "chat-a", "prompt-requests"), dirA)
}

// TestPromptJournalDirFor_PropagatesHomeFailure: the derivation is best-effort
// on crowbar home, exactly like every other path-deriving call in this
// package — a resolver failure must surface as an error, not a panic or a
// silently wrong path.
func TestPromptJournalDirFor_PropagatesHomeFailure(t *testing.T) {
	boom := errors.New("boom: crowbar home")
	rs := &Runners{home: func() (string, error) { return "", boom }}

	_, err := rs.promptJournalDirFor("chat-1")
	require.ErrorIs(t, err, boom)
}

type stubChatsForSettle struct {
	agentchat.EventStore
	chat domain.Chat
}

func (s stubChatsForSettle) GetChat(context.Context, string) (domain.Chat, error) {
	return s.chat, nil
}

// stubConversationsForSettle answers ChatTurns with an EMPTY ledger — the whole
// point of the case under test is a prompt that produced no turn anywhere.
type stubConversationsForSettle struct {
	Conversations
}

func (stubConversationsForSettle) ChatTurns(
	context.Context, string,
) ([]domain.LedgerTurn, error) {
	return nil, nil
}

type settledCall struct {
	chatID      string
	workspaceID string
	requestID   string
	consumed    bool
}

// settleFixture builds a Runners with a real on-disk prompt journal holding one
// delivery in the `spawned` state — the only state Settle retires, and the state
// a prompt the daemon accepted but that never produced a turn is left in.
func settleFixture(t *testing.T) (*Runners, *[]settledCall) {
	t.Helper()
	home := t.TempDir()
	chatsDir := filepath.Join(home, "chats")
	var calls []settledCall

	rs := &Runners{
		home:          func() (string, error) { return home, nil },
		ws:            fakeWSReader{chatsDir: chatsDir},
		chats:         stubChatsForSettle{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}},
		conversations: stubConversationsForSettle{},
		prompts:       agentjournal.NewPromptRequests(),
	}
	rs.promptSettled = func(chatID, workspaceID, requestID string, consumed bool) {
		calls = append(calls, settledCall{chatID, workspaceID, requestID, consumed})
	}

	dir := rs.prompts.Dir(chatsDir, "chat-1")
	now := time.Now()
	_, existing, err := rs.prompts.Begin(
		dir, "req-1", "", "hash-1", "codex", "runner-1", "runner-1", now,
	)
	require.NoError(t, err)
	require.False(t, existing)
	_, err = rs.prompts.MarkSpawned(dir, "req-1", "hash-1", "runner-1", "term-1", now)
	require.NoError(t, err)

	return rs, &calls
}

// TestRegression_SettleDelivery_TimeoutReportsThePromptWasNotConsumed is the
// data-loss bug, at the seam where it is decidable.
//
// Reported live against codex: "User's turns after some time of idle is lost,
// and does not record anywhere." A prompt the daemon accepts but that never
// produces a turn is retired by the terminal-wait sweep after thirty seconds of
// a quiet screen, and that retirement is announced to the browser. The browser's
// pending-queue item is at that moment the ONLY copy of the user's text in the
// entire system — this journal stores a hash of it and never the text, and
// nothing reached the ledger — so an announcement that says merely "this is
// over" is read as permission to delete the user's words.
//
// The sweep has no evidence of anything. It must say so.
func TestRegression_SettleDelivery_TimeoutReportsThePromptWasNotConsumed(t *testing.T) {
	rs, calls := settleFixture(t)

	retired, err := rs.SettleDelivery(context.Background(), "chat-1", "req-1")
	require.NoError(t, err)
	require.True(t, retired, "a spawned delivery that produced no turn is the sweep's to retire")

	require.Equal(t, []settledCall{
		{chatID: "chat-1", workspaceID: "ws-1", requestID: "req-1", consumed: false},
	}, *calls)
}

// TestRegression_SettleDeliveryFor_ReportsTheProviderConsumedThePrompt is the
// other half, and the reason the flag exists at all rather than the client
// simply never dropping an unproven prompt.
//
// compact_post runs this one: the CLI demonstrably ACTED on the prompt — it
// compacted — and a provider built-in announces no turn by design. That text is
// genuinely spent, and a client that kept it would strand a dead row in the
// composer forever. The record on disk is byte-identical in both cases, so only
// the call site can tell them apart.
func TestRegression_SettleDeliveryFor_ReportsTheProviderConsumedThePrompt(t *testing.T) {
	rs, calls := settleFixture(t)

	require.NoError(t, rs.SettleDeliveryFor(context.Background(), "chat-1", "runner-1"))

	require.Equal(t, []settledCall{
		{chatID: "chat-1", workspaceID: "ws-1", requestID: "req-1", consumed: true},
	}, *calls)
}
