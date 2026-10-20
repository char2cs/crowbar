package turn

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// noticeRecords keeps each notice a failed turn records.
type noticeRecords struct {
	Conversations
	mu      sync.Mutex
	notices []string
}

func (n *noticeRecords) RecordTurn(_ context.Context, _ domain.Chat, _, _, _, _, _, text, _ string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.notices = append(n.notices, text)
	return nil
}

func (n *noticeRecords) recorded() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.notices...)
}

type abandonActivity struct{ agentactivity.EventStore }

func (abandonActivity) Abandon(context.Context, string, time.Time) error { return nil }

// A notice after a hook ended the turn has no turn to fail, and nothing to explain.
func TestFailTurnOnNotice_AnEndedTurnLeavesNothingToFail(t *testing.T) {
	l := newTurnLedger(t)
	appendTurnRows(t, l, turnRow(t, "runner-1"))
	notices := &noticeRecords{}
	turns := New(Deps{Ledger: l, Chats: raceChats{}, Activity: abandonActivity{}, Conversations: notices})
	runner := engineagents.Runner{ID: "runner-1", ProviderID: "codex", CurrentChatID: "chat-1"}
	st, err := l.State(t.Context(), "chat-1")
	require.NoError(t, err)
	ended, err := ledger.EndTurn(st, ledger.TurnStatusDone, "")
	require.NoError(t, err)
	appendTurnRows(t, l, ended...)

	turns.FailTurnOnNotice(context.Background(), runner,
		engineagents.TerminalNotice{Kind: "usage_limit", Text: "You've hit your usage limit"})

	rows, err := l.Read(t.Context(), "chat-1", 0, 0)
	require.NoError(t, err)
	for _, r := range rows {
		assert.NotEqual(t, ledger.KindTurnFailed, r.Kind, "the turn a hook ended is not failed after it")
	}
	assert.Empty(t, notices.recorded(), "and no notice explains an end that did not happen")
}

// A notice on the screen of a CLI that never took the prompt it carries
// settles that prompt: the provider will not take it now.
func TestFailTurnOnNotice_SettlesAnAwaitedPrompt(t *testing.T) {
	l := newTurnLedger(t)
	appendTurnRows(t, l, carriedPrompt(t, "req-1", "runner-1", ledger.DeliveryRestart))
	notices := &noticeRecords{}
	turns := New(Deps{Ledger: l, Chats: raceChats{}, Activity: abandonActivity{}, Conversations: notices})

	turns.FailTurnOnNotice(t.Context(),
		engineagents.Runner{ID: "runner-1", ProviderID: "codex", CurrentChatID: "chat-1"},
		engineagents.TerminalNotice{Kind: "usage_limit", Text: "You've hit your usage limit"})

	st, err := l.State(t.Context(), "chat-1")
	require.NoError(t, err)
	got, ok := st.Receipt("req-1")
	require.True(t, ok)
	assert.Equal(t, ledger.PromptStatusSettled, got.Status)
	assert.False(t, got.Consumed)
	assert.Equal(t, "usage_limit", got.Reason)
	assert.Equal(t, []string{"You've hit your usage limit"}, notices.recorded(), "the notice says why")
}
