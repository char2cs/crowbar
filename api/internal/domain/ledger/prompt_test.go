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

func TestSettle_SettlesOnlyThePendingPromptsItSelects(t *testing.T) {
	carried := func(runnerID string) func(ledger.Prompt) bool {
		return func(p ledger.Prompt) bool { return p.RunnerID == runnerID }
	}
	msg := func(requestID, runnerID, turnID string) ledger.Row {
		return row(t, ledger.KindUserMessage, turnID, ledger.UserMessage{
			RequestID: requestID, Text: requestID, Origin: ledger.OriginCrowbar, RunnerID: runnerID,
		})
	}
	st := ledger.Fold("c1", sequenced(t,
		msg("r1", "run-1", "T1"),
		msg("r2", "run-2", "T2"),
		msg("r3", "run-1", "T3"),
		row(t, ledger.KindTurnStarted, "T3", ledger.TurnStarted{RequestID: "r3"}),
	))

	rows, err := ledger.Settle(st, carried("run-1"), true, ledger.ReasonProviderBuiltin)
	require.NoError(t, err)
	require.Len(t, rows, 1, "r3 is delivered and r2 is carried by another runner")
	body, err := ledger.DecodeBody[ledger.PromptSettled](rows[0])
	require.NoError(t, err)
	assert.Equal(t, ledger.PromptSettled{RequestID: "r1", Consumed: true, Reason: "provider_builtin"}, body)

	none, err := ledger.Settle(st, carried("run-9"), false, ledger.ReasonRunnerExited)
	require.NoError(t, err)
	assert.Empty(t, none)
}
