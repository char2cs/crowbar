package ledger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func TestConversationState_Next(t *testing.T) {
	states := []ledger.ConversationState{ledger.ConversationUnbound, ledger.ConversationBound, ledger.ConversationLost}
	allowed := map[[2]ledger.ConversationState]bool{
		{ledger.ConversationUnbound, ledger.ConversationBound}: true,
		{ledger.ConversationBound, ledger.ConversationLost}:    true,
		{ledger.ConversationLost, ledger.ConversationBound}:    true,
		{ledger.ConversationBound, ledger.ConversationBound}:   true,
	}
	for _, from := range states {
		for _, to := range states {
			got, ok := from.Next(to)
			if allowed[[2]ledger.ConversationState{from, to}] {
				assert.True(t, ok, "%s→%s", from, to)
				assert.Equal(t, to, got, "%s→%s", from, to)
				continue
			}
			assert.False(t, ok, "%s→%s", from, to)
			assert.Equal(t, from, got, "a refused transition stays where it was: %s→%s", from, to)
		}
	}
}

func decodeRows(t *testing.T, rows []ledger.Row) []string {
	t.Helper()
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		require.NoError(t, ledger.Check(ledger.State{}, r))
		switch r.Kind {
		case ledger.KindSessionBound:
			b, err := ledger.DecodeBody[ledger.SessionBound](r)
			require.NoError(t, err)
			out = append(out, "bound "+b.SessionID+" "+b.Cause+" after "+b.Previous)
		case ledger.KindMarker:
			m, err := ledger.DecodeBody[ledger.Marker](r)
			require.NoError(t, err)
			out = append(out, "marker "+m.Kind+" "+m.Detail)
		case ledger.KindUserMessage, ledger.KindTurnStarted, ledger.KindPromptSettled, ledger.KindSessionLost,
			ledger.KindRunnerStarted, ledger.KindRunnerExited:
			out = append(out, string(r.Kind))
		}
	}
	return out
}

func TestAssign_IsFreshOnlyAfterALoss(t *testing.T) {
	first, err := ledger.Assign(ledger.State{}, "p1", "S1")
	require.NoError(t, err)
	assert.Equal(t, []string{"bound S1 assigned after "}, decodeRows(t, []ledger.Row{first}))

	st := ledger.Fold("c1", sequenced(t, bound(t, "p1", "S1", ledger.CauseAssigned, ""), lostS1(t, "p1")))
	again, err := ledger.Assign(st, "p1", "S2")
	require.NoError(t, err)
	assert.Equal(t, []string{"bound S2 fresh after S1"}, decodeRows(t, []ledger.Row{again}))

	again.Seq = 3
	st.Apply(again)
	c, _ := st.Conversation("p1")
	assert.Equal(t, ledger.CauseFresh, c.Cause, "the conversation remembers it replaced a lost one")

	stillFresh, err := ledger.Assign(st, "p1", "S3")
	require.NoError(t, err)
	assert.Equal(t, []string{"bound S3 fresh after S2"}, decodeRows(t, []ledger.Row{stillFresh}),
		"a fresh session that never turned is replaced by another fresh one")

	unturned := ledger.Fold("c1", sequenced(t, bound(t, "p1", "S1", ledger.CauseAssigned, "")))
	replaced, err := ledger.Assign(unturned, "p1", "S2")
	require.NoError(t, err)
	assert.Equal(t, []string{"bound S2 assigned after S1"}, decodeRows(t, []ledger.Row{replaced}))
}

func TestAnnounce_DecidesBetweenNothingABindingAChangeAndAConflict(t *testing.T) {
	boundS1 := ledger.Fold("c1", sequenced(t, bound(t, "p1", "S1", ledger.CauseAssigned, "")))
	turnedS1 := ledger.Fold("c1", sequenced(t, bound(t, "p1", "S1", ledger.CauseAssigned, ""), turnedOn(t, "p1", "S1")))
	lostS1 := ledger.Fold("c1", sequenced(t, bound(t, "p1", "S1", ledger.CauseAssigned, ""), lostS1(t, "p1")))
	cases := []struct {
		name    string
		st      ledger.State
		id      string
		claimed bool
		want    []string
	}{
		{"the bound session", boundS1, "S1", true, []string{}},
		{"an unbound chat", ledger.State{}, "S1", false, []string{"bound S1 announced after "}},
		{"a lost chat", lostS1, "S2", false, []string{"bound S2 announced after S1"}},
		{"a binding that never turned", boundS1, "S2", false, []string{"bound S2 announced after S1"}},
		{"a change", turnedS1, "S2", false, []string{"bound S2 announced after S1", "marker session_changed S2"}},
		{"another chat's session", turnedS1, "S2", true, []string{"marker session_conflict S2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := ledger.Announce(tc.st, "p1", tc.id, tc.claimed)
			require.NoError(t, err)
			assert.Equal(t, tc.want, decodeRows(t, rows))
		})
	}
}

func TestLose_LosesTheBoundSessionAndSettlesItsCarriersPrompt(t *testing.T) {
	carried := row(t, ledger.KindUserMessage, "T1", ledger.UserMessage{
		RequestID: "r1", Text: "a", Origin: ledger.OriginCrowbar, Delivery: ledger.DeliveryRestart, RunnerID: "run-1",
	})
	st := ledger.Fold("c1", sequenced(t, bound(t, "p1", "S1", ledger.CauseAssigned, ""), carried))

	rows, err := ledger.Lose(st, "p1", "S1", "No conversation found", "run-1")
	require.NoError(t, err)
	require.Equal(t, []string{"session_lost", "prompt_settled"}, decodeRows(t, rows))
	settled, err := ledger.DecodeBody[ledger.PromptSettled](rows[1])
	require.NoError(t, err)
	assert.Equal(t, ledger.PromptSettled{RequestID: "r1", Reason: ledger.ReasonSessionLost}, settled)

	for _, rows := range [][]ledger.Row{
		mustLose(t, st, "p1", "S0"),
		mustLose(t, ledger.State{}, "p1", "S1"),
		mustLose(t, ledger.Fold("c1", sequenced(t, bound(t, "p1", "S1", ledger.CauseAssigned, ""), lostS1(t, "p1"))), "p1", "S1"),
	} {
		assert.Empty(t, rows, "only the bound session can be lost")
	}
}

func mustLose(t *testing.T, st ledger.State, provider, session string) []ledger.Row {
	t.Helper()
	rows, err := ledger.Lose(st, provider, session, "refused", "run-1")
	require.NoError(t, err)
	return rows
}

func TestCheck_RefusesASessionRowWithoutItsIdentity(t *testing.T) {
	for name, r := range map[string]ledger.Row{
		"bound without a session": bound(t, "p1", "", ledger.CauseAssigned, ""),
		"bound without a cause":   bound(t, "p1", "S1", "", ""),
		"lost without a provider": lostS1(t, ""),
		"an undecodable binding":  {Kind: ledger.KindSessionBound, Body: []byte("{")},
	} {
		assert.ErrorIs(t, ledger.Check(ledger.State{}, r), ledger.ErrMalformedRow, name)
	}
}
