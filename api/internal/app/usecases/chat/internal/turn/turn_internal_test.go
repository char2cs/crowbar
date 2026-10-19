package turn

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// appendTurnRows records rows on chat-1.
func appendTurnRows(t *testing.T, l *ledgerstore.Store, rows ...ledger.Row) {
	t.Helper()
	_, _, err := l.Append(t.Context(), "chat-1", rows...)
	require.NoError(t, err)
}

func turnRow(t *testing.T, runnerID string) ledger.Row {
	t.Helper()
	row, err := ledger.NewRow(ledger.KindTurnStarted, "T-"+runnerID, "", ledger.TurnStarted{RunnerID: runnerID})
	require.NoError(t, err)
	return row
}

func carriedPrompt(t *testing.T, requestID, runnerID, delivery string) ledger.Row {
	t.Helper()
	row, err := ledger.NewRow(ledger.KindUserMessage, "T-"+requestID, "", ledger.UserMessage{
		RequestID: requestID, Text: requestID, Origin: ledger.OriginCrowbar, RunnerID: runnerID, Delivery: delivery,
	})
	require.NoError(t, err)
	return row
}

// A provider saying it is idle closes the turn its runner is running at once,
// and the steer it carried for that turn is never delivered.
func TestRecordIdle_EndsTheRunnersRunningTurn(t *testing.T) {
	l := newTurnLedger(t)
	appendTurnRows(t, l, turnRow(t, "runner-1"), carriedPrompt(t, "steer-1", "runner-1", ledger.DeliverySteer))
	turns := New(Deps{Ledger: l})

	turns.recordIdle(t.Context(), "chat-1", engineagents.Runner{ID: "runner-1"})

	st, err := l.State(t.Context(), "chat-1")
	require.NoError(t, err)
	assert.False(t, st.Busy())
	got, ok := st.Receipt("steer-1")
	require.True(t, ok)
	assert.Equal(t, ledger.ReasonDeliveryFailed, got.Reason)
}

// An idle report from a runner the running turn is not on is written late,
// and says nothing about that turn.
func TestRecordIdle_LeavesAnotherRunnersTurnRunning(t *testing.T) {
	l := newTurnLedger(t)
	appendTurnRows(t, l, turnRow(t, "runner-2"))
	turns := New(Deps{Ledger: l})

	turns.recordIdle(t.Context(), "chat-1", engineagents.Runner{ID: "runner-1"})

	st, err := l.State(t.Context(), "chat-1")
	require.NoError(t, err)
	assert.True(t, st.Busy())
	rows, err := l.Read(t.Context(), "chat-1", 1, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, ledger.KindTurnEnded, rows[0].Kind)
	assert.NotEqual(t, "T-runner-2", rows[0].TurnID)
}

// A provider that says it is idle before any turn started never took the
// prompt its CLI carries.
func TestRecordIdle_SettlesTheCarriedPromptWhenNoTurnStarted(t *testing.T) {
	l := newTurnLedger(t)
	appendTurnRows(t, l, carriedPrompt(t, "req-1", "runner-1", ledger.DeliveryRestart))
	turns := New(Deps{Ledger: l})

	turns.recordIdle(t.Context(), "chat-1", engineagents.Runner{ID: "runner-1"})

	st, err := l.State(t.Context(), "chat-1")
	require.NoError(t, err)
	got, ok := st.Receipt("req-1")
	require.True(t, ok)
	assert.Equal(t, ledger.PromptStatusSettled, got.Status)
	assert.False(t, got.Consumed)
	assert.Equal(t, ledger.ReasonNotAcknowledged, got.Reason)
}
