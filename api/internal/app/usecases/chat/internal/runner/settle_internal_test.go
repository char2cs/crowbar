package runner

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

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
	chatsDir := filepath.Join(t.TempDir(), "chats")
	var calls []settledCall

	rs := &Runners{
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
