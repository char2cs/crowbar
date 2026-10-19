package turn_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// claudeTurns is the hook ingress over l for claude runners its live table
// still holds open, whatever the ledger says of them.
func claudeTurns(t *testing.T, l *ledgerstore.Store) *turn.Turns {
	t.Helper()
	home := t.TempDir()
	turns := turn.New(turn.Deps{
		Chats:         stubChats{},
		Ledger:        l,
		Activity:      &recordingTranscript{ledger: l},
		Conversations: quietConversations{},
		Agents:        engineagents.New(),
		Workspace:     stubWorkspace{home: home},
		Home:          func() (string, error) { return home, nil },
		PendingHooks:  inflight.NewHooks(),
		Telemetry:     telemetry.New(),
	})
	turns.SetRunners(quietRunners{claudeLive})
	return turns
}

func ledgerRow(t *testing.T, kind ledger.Kind, turnID string, body any) ledger.Row {
	t.Helper()
	r, err := ledger.NewRow(kind, turnID, "", body)
	require.NoError(t, err)
	return r
}

func chatState(t *testing.T, l *ledgerstore.Store) ledger.State {
	t.Helper()
	st, err := l.State(t.Context(), "chat-1")
	require.NoError(t, err)
	return st
}

// A prompt acknowledged by a CLI the ledger no longer holds live starts no
// turn: nothing would ever close it.
func TestStartTurn_APromptOffTheLiveRunnerWritesNothing(t *testing.T) {
	t.Parallel()

	runner := func(kind ledger.Kind, id string) ledger.Row {
		if kind == ledger.KindRunnerStarted {
			return ledgerRow(t, kind, "", ledger.RunnerStarted{RunnerID: id, Provider: "claude"})
		}
		return ledgerRow(t, kind, "", ledger.RunnerExited{RunnerID: id, Reason: "exited"})
	}
	for name, rows := range map[string][]ledger.Row{
		"its runner exited": {runner(ledger.KindRunnerStarted, "runner-1"), runner(ledger.KindRunnerExited, "runner-1")},
		"another runner is live": {
			runner(ledger.KindRunnerStarted, "runner-1"), runner(ledger.KindRunnerExited, "runner-1"),
			runner(ledger.KindRunnerStarted, "runner-2"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			l := emptyLedger(t)
			_, before, err := l.Append(t.Context(), "chat-1", rows...)
			require.NoError(t, err)

			require.NoError(t, claudeTurns(t, l).IngestHook(t.Context(), "runner-1", "claude", "user_prompt",
				[]byte(`{"session_id":"s-1","prompt":"typed at the CLI"}`)))

			assert.Equal(t, before.Head(), chatState(t, l).Head(), "a runner that is not live writes no rows")
		})
	}
}

// A turn_stop ending its runner's turn delivers the steer it carried only
// when that runner is still the live one; otherwise the steer is refused.
func TestStartTurn_ASteerOffTheLiveRunnerIsRefused(t *testing.T) {
	t.Parallel()

	l := emptyLedger(t)
	_, _, err := l.Append(t.Context(), "chat-1",
		ledgerRow(t, ledger.KindRunnerStarted, "", ledger.RunnerStarted{RunnerID: "runner-1", Provider: "claude"}),
		ledgerRow(t, ledger.KindTurnStarted, "T-1", ledger.TurnStarted{RunnerID: "runner-1"}),
		ledgerRow(t, ledger.KindUserMessage, "T-2", ledger.UserMessage{
			RequestID: "steer-1", Text: "and this too", Origin: ledger.OriginCrowbar,
			RunnerID: "runner-1", Delivery: ledger.DeliverySteer,
		}),
		ledgerRow(t, ledger.KindRunnerStarted, "", ledger.RunnerStarted{RunnerID: "runner-2", Provider: "claude"}),
	)
	require.NoError(t, err)

	require.NoError(t, claudeTurns(t, l).IngestHookDelivery(t.Context(), uuid.NewString(), "runner-1", "claude",
		"turn_stop", []byte(`{"session_id":"s-1","last_assistant_message":""}`)))

	st := chatState(t, l)
	assert.False(t, st.Busy(), "the steer opened no turn on a runner that is not live")
	got, ok := st.Receipt("steer-1")
	require.True(t, ok)
	assert.Equal(t, ledger.PromptStatusSettled, got.Status)
	assert.Equal(t, ledger.ReasonDeliveryFailed, got.Reason)
}
