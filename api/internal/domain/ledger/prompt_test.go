package ledger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func TestReceipt_CarriesWhatTheUserMessageRecorded(t *testing.T) {
	msg := row(t, ledger.KindUserMessage, "T1", ledger.UserMessage{
		RequestID: "r1", Text: "hello", Origin: ledger.OriginCrowbar,
		Delivery: ledger.DeliverySteer, RunnerID: "run-1",
	})
	st := ledger.Fold("c1", sequenced(t, msg))

	got, ok := st.Receipt("r1")
	require.True(t, ok)
	assert.Equal(t, ledger.Prompt{
		RequestID: "r1", TurnID: "T1", Text: "hello", Delivery: "steer", RunnerID: "run-1",
		Status: ledger.PromptStatusPending, At: msg.At,
	}, got)
}

func TestReceipt_RecordsAConsumedSettle(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		crowbarPrompt(t, "r1", "/compact", "T1"),
		row(t, ledger.KindPromptSettled, "", ledger.PromptSettled{
			RequestID: "r1", Consumed: true, Reason: ledger.ReasonProviderBuiltin,
		}),
	))

	got, ok := st.Receipt("r1")
	require.True(t, ok)
	assert.Equal(t, ledger.PromptStatusSettled, got.Status)
	assert.True(t, got.Consumed)
	assert.Equal(t, ledger.ReasonProviderBuiltin, got.Reason)
}

// carriedPrompts is r1 and r3 on run-1 (r3 delivered), r2 on run-2, and r4
// with no carrier.
func carriedPrompts(t *testing.T) ledger.State {
	t.Helper()
	msg := func(requestID, runnerID, turnID string) ledger.Row {
		return row(t, ledger.KindUserMessage, turnID, ledger.UserMessage{
			RequestID: requestID, Text: requestID, Origin: ledger.OriginCrowbar, RunnerID: runnerID,
		})
	}
	return ledger.Fold("c1", sequenced(t,
		msg("r1", "run-1", "T1"),
		msg("r2", "run-2", "T2"),
		msg("r3", "run-1", "T3"),
		row(t, ledger.KindTurnStarted, "T3", ledger.TurnStarted{RequestID: "r3"}),
		msg("r4", "", "T4"),
	))
}

func settled(t *testing.T, rows []ledger.Row) []ledger.PromptSettled {
	t.Helper()
	out := make([]ledger.PromptSettled, 0, len(rows))
	for _, r := range rows {
		body, err := ledger.DecodeBody[ledger.PromptSettled](r)
		require.NoError(t, err)
		out = append(out, body)
	}
	return out
}

func TestSettle_SettlesOnlyThePendingPromptsItSelects(t *testing.T) {
	st := carriedPrompts(t)
	first := func(p ledger.Prompt) bool { return p.RequestID == "r1" || p.RequestID == "r3" }

	rows, err := ledger.Settle(st, first, true, ledger.ReasonProviderBuiltin)
	require.NoError(t, err)
	assert.Equal(t, []ledger.PromptSettled{{RequestID: "r1", Consumed: true, Reason: "provider_builtin"}},
		settled(t, rows), "r3 is delivered")
}

func TestSettleCarried_SettlesOnlyWhatThatRunnerCarries(t *testing.T) {
	st := carriedPrompts(t)

	rows, err := ledger.SettleCarried(st, "run-1", false, ledger.ReasonRunnerExited)
	require.NoError(t, err)
	assert.Equal(t, []ledger.PromptSettled{{RequestID: "r1", Reason: "runner_exited"}}, settled(t, rows))

	none, err := ledger.SettleCarried(st, "run-9", false, ledger.ReasonRunnerExited)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestSettleOrphaned_SettlesPromptsWhoseCarrierIsNotLive(t *testing.T) {
	st := carriedPrompts(t)
	live := func(runnerID string) bool { return runnerID == "run-1" }

	rows, err := ledger.SettleOrphaned(st, live)
	require.NoError(t, err)
	assert.Equal(t, []ledger.PromptSettled{{RequestID: "r2", Reason: "runner_exited"}}, settled(t, rows),
		"run-1 is live, and a prompt with no carrier is never orphaned")
}
