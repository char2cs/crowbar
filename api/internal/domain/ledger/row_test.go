package ledger_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func TestCheck_RefusesMalformedRows(t *testing.T) {
	message, err := ledger.NewRow(ledger.KindUserMessage, "T1", "",
		ledger.UserMessage{RequestID: "r1", Text: "a", Origin: ledger.OriginCrowbar})
	require.NoError(t, err)
	started, err := ledger.NewRow(ledger.KindTurnStarted, "T1", "", ledger.TurnStarted{RequestID: "r1"})
	require.NoError(t, err)
	settled, err := ledger.NewRow(ledger.KindPromptSettled, "", "", ledger.PromptSettled{RequestID: "r1"})
	require.NoError(t, err)

	without := func(r ledger.Row, edit func(*ledger.Row)) ledger.Row {
		edit(&r)
		return r
	}
	cases := []struct {
		name string
		row  ledger.Row
		ok   bool
	}{
		{"a well-formed user_message", message, true},
		{"a well-formed turn_started", started, true},
		{"a chat-level prompt_settled", settled, true},
		{"an unknown kind", without(message, func(r *ledger.Row) { r.Kind = "assistant_shout" }), false},
		{"a user_message without a turn", without(message, func(r *ledger.Row) { r.TurnID = "" }), false},
		{"a turn_started without a turn", without(started, func(r *ledger.Row) { r.TurnID = "" }), false},
		{"an undecodable user_message", without(message, func(r *ledger.Row) { r.Body = json.RawMessage("{") }), false},
		{"an undecodable prompt_settled", without(settled, func(r *ledger.Row) { r.Body = json.RawMessage("{") }), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ledger.Check(ledger.State{}, tc.row)
			if tc.ok {
				assert.NoError(t, err)
				return
			}
			assert.Error(t, err)
		})
	}
}

func TestDecodeBody_RoundTripsWhatNewRowEncoded(t *testing.T) {
	row, err := ledger.NewRow(ledger.KindPromptSettled, "", "",
		ledger.PromptSettled{RequestID: "r1", Consumed: true, Reason: ledger.ReasonProviderBuiltin})
	require.NoError(t, err)

	got, err := ledger.DecodeBody[ledger.PromptSettled](row)
	require.NoError(t, err)
	assert.Equal(t, ledger.PromptSettled{RequestID: "r1", Consumed: true, Reason: "provider_builtin"}, got)

	_, err = ledger.DecodeBody[ledger.PromptSettled](ledger.Row{Body: json.RawMessage("[")})
	assert.Error(t, err)
}

func TestNewRow_RefusesABodyThatCannotBeEncoded(t *testing.T) {
	_, err := ledger.NewRow(ledger.KindUserMessage, "T1", "", make(chan int))
	assert.Error(t, err)
}
