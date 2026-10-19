package ledger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// sequenced numbers rows from 1 the way the store does.
func sequenced(t *testing.T, rows ...ledger.Row) []ledger.Row {
	t.Helper()
	for i := range rows {
		rows[i].ChatID = "c1"
		rows[i].Seq = int64(i + 1)
	}
	return rows
}

func row[B any](t *testing.T, kind ledger.Kind, turnID string, body B) ledger.Row {
	t.Helper()
	r, err := ledger.NewRow(kind, turnID, "", body)
	require.NoError(t, err)
	return r
}

func crowbarPrompt(t *testing.T, requestID, text, turnID string) ledger.Row {
	t.Helper()
	return row(t, ledger.KindUserMessage, turnID, ledger.UserMessage{
		RequestID: requestID, Text: text, Origin: ledger.OriginCrowbar, Delivery: ledger.DeliveryRestart,
	})
}

func TestFold_PromptLifecycle(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		crowbarPrompt(t, "r1", "a", "T1"),
		row(t, ledger.KindTurnStarted, "T1", ledger.TurnStarted{RequestID: "r1", RunnerID: "run-1"}),
		row(t, ledger.KindTurnEnded, "T1", ledger.TurnEnded{Status: "done"}),
		crowbarPrompt(t, "r2", "b", "T2"),
		row(t, ledger.KindPromptSettled, "", ledger.PromptSettled{RequestID: "r2", Reason: ledger.ReasonRunnerExited}),
	))

	first, ok := st.Receipt("r1")
	require.True(t, ok)
	assert.Equal(t, ledger.PromptStatusDelivered, first.Status)
	assert.Equal(t, "T1", first.TurnID)
	assert.Equal(t, "a", first.Text)

	second, ok := st.Receipt("r2")
	require.True(t, ok)
	assert.Equal(t, ledger.PromptStatusSettled, second.Status)
	assert.False(t, second.Consumed)
	assert.Equal(t, "runner_exited", second.Reason)

	assert.Empty(t, st.PendingPrompts())
	assert.False(t, st.HasOpenWork())
	assert.Equal(t, int64(5), st.Head())
	assert.Equal(t, "c1", st.ChatID)
	_, ok = st.Receipt("never-sent")
	assert.False(t, ok)
}

func TestFold_ProviderAndHarnessOriginLeaveTheCrowbarPromptPending(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		crowbarPrompt(t, "r1", "a", "T1"),
		row(t, ledger.KindUserMessage, "T2", ledger.UserMessage{Text: "typed", Origin: ledger.OriginProvider}),
		row(t, ledger.KindTurnStarted, "T2", ledger.TurnStarted{RunnerID: "run-1"}),
		row(t, ledger.KindUserMessage, "T3", ledger.UserMessage{Text: "<agent-message>", Origin: ledger.OriginHarness}),
		row(t, ledger.KindTurnStarted, "T3", ledger.TurnStarted{RunnerID: "run-1"}),
	))

	pending := st.PendingPrompts()
	require.Len(t, pending, 1)
	assert.Equal(t, "r1", pending[0].RequestID)
	assert.Equal(t, ledger.PromptStatusPending, pending[0].Status)
	assert.True(t, st.HasOpenWork())
	_, ok := st.Receipt("")
	assert.False(t, ok, "a provider or harness message is no crowbar prompt")
}

func TestFold_PendingPromptsAreOldestFirst(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		crowbarPrompt(t, "r1", "a", "T1"),
		crowbarPrompt(t, "r2", "b", "T2"),
		crowbarPrompt(t, "r3", "c", "T3"),
		row(t, ledger.KindTurnStarted, "T2", ledger.TurnStarted{RequestID: "r2"}),
	))

	pending := st.PendingPrompts()
	require.Len(t, pending, 2)
	assert.Equal(t, []string{"r1", "r3"}, []string{pending[0].RequestID, pending[1].RequestID})
}

func TestFold_LateRowsChangeNothing(t *testing.T) {
	rows := sequenced(t,
		crowbarPrompt(t, "r1", "a", "T1"),
		row(t, ledger.KindTurnStarted, "T1", ledger.TurnStarted{RequestID: "r1"}),
		row(t, ledger.KindTurnStarted, "T9", ledger.TurnStarted{RequestID: "unknown"}),
		row(t, ledger.KindPromptSettled, "", ledger.PromptSettled{RequestID: "r1", Reason: ledger.ReasonRunnerExited}),
		row(t, ledger.KindPromptSettled, "", ledger.PromptSettled{RequestID: "unknown"}),
		crowbarPrompt(t, "r1", "a different text", "T5"),
	)
	st := ledger.Fold("c1", rows)

	got, ok := st.Receipt("r1")
	require.True(t, ok)
	assert.Equal(t, ledger.PromptStatusDelivered, got.Status)
	assert.Equal(t, "a", got.Text)
	assert.Equal(t, "T1", got.TurnID)
	assert.Empty(t, got.Reason)
	_, ok = st.Receipt("unknown")
	assert.False(t, ok)

	assert.NotPanics(t, func() {
		st.Apply(ledger.Row{Seq: 7, Kind: "assistant_shout"})
		st.Apply(ledger.Row{Seq: 8, Kind: ledger.KindUserMessage, Body: []byte("{")})
		st.Apply(ledger.Row{Seq: 9, Kind: ledger.KindTurnStarted, Body: []byte("{")})
		st.Apply(ledger.Row{Seq: 10, Kind: ledger.KindPromptSettled, Body: []byte("{")})
	})
	assert.Equal(t, int64(10), st.Head())
	assert.Empty(t, st.PendingPrompts())
}

func TestState_CloneIsIndependentOfTheOriginal(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t, crowbarPrompt(t, "r1", "a", "T1")))
	snapshot := st.Clone()

	st.Apply(ledger.Row{
		Seq: 2, Kind: ledger.KindTurnStarted, TurnID: "T1", Body: []byte(`{"requestId":"r1"}`),
	})

	got, ok := snapshot.Receipt("r1")
	require.True(t, ok)
	assert.Equal(t, ledger.PromptStatusPending, got.Status, "a clone must not see rows applied to its original")
	assert.Len(t, snapshot.PendingPrompts(), 1)
	assert.Equal(t, int64(1), snapshot.Head())
}

func bound(t *testing.T, provider, session, cause, previous string) ledger.Row {
	t.Helper()
	return row(t, ledger.KindSessionBound, "", ledger.SessionBound{
		Provider: provider, SessionID: session, Cause: cause, Previous: previous,
	})
}

// turnedOn is a turn the provider started on session.
func turnedOn(t *testing.T, provider, session string) ledger.Row {
	t.Helper()
	return row(t, ledger.KindTurnStarted, "T-"+session, ledger.TurnStarted{Provider: provider, SessionID: session})
}

// lostS1 is provider's session S1 refused by its CLI.
func lostS1(t *testing.T, provider string) ledger.Row {
	t.Helper()
	return row(t, ledger.KindSessionLost, "", ledger.SessionLost{Provider: provider, SessionID: "S1", Reason: "refused"})
}

func TestFold_ConversationPerProvider(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		bound(t, "p1", "S1", ledger.CauseAssigned, ""),
		bound(t, "p2", "X1", ledger.CauseAnnounced, ""),
		lostS1(t, "p1"),
		bound(t, "p1", "S2", ledger.CauseFresh, "S1"),
	))

	p1, ok := st.Conversation("p1")
	require.True(t, ok)
	assert.Equal(t, "S2", p1.SessionID)
	assert.Equal(t, ledger.ConversationBound, p1.State)
	p2, ok := st.Conversation("p2")
	require.True(t, ok)
	assert.Equal(t, "X1", p2.SessionID)
	assert.Equal(t, ledger.ConversationBound, p2.State)
	_, ok = st.Conversation("p3")
	assert.False(t, ok, "a provider the chat never ran has no conversation")

	var sessions []string
	var states []ledger.ConversationState
	for _, c := range st.Conversations() {
		sessions = append(sessions, c.SessionID)
		states = append(states, c.State)
	}
	assert.Equal(t, []string{"S1", "X1", "S2"}, sessions)
	assert.Equal(t, []ledger.ConversationState{ledger.ConversationLost, ledger.ConversationBound, ledger.ConversationBound}, states)
}

func TestFold_AConversationHasTurnedOnceATurnStartsOnIt(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		bound(t, "p1", "S1", ledger.CauseAssigned, ""),
		bound(t, "p2", "X1", ledger.CauseAnnounced, ""),
		turnedOn(t, "p1", "S1"),
		row(t, ledger.KindTurnStarted, "T-none", ledger.TurnStarted{}),
		bound(t, "p1", "S2", ledger.CauseAnnounced, "S1"),
		bound(t, "p1", "S1", ledger.CauseAnnounced, "S2"),
	))

	p1, _ := st.Conversation("p1")
	assert.True(t, p1.Turned, "a session keeps having turned when the chat returns to it")
	p2, _ := st.Conversation("p2")
	assert.False(t, p2.Turned, "a turn names its own conversation only")
	assert.False(t, st.Conversations()[2].Turned, "S2 was bound but never turned")
}

func TestFold_ASessionLossOnlyLosesTheBoundSession(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		bound(t, "p1", "S1", ledger.CauseAssigned, ""),
		bound(t, "p1", "S2", ledger.CauseAnnounced, "S1"),
		lostS1(t, "p1"),
		lostS1(t, "p9"),
		bound(t, "p1", "S1", ledger.CauseAnnounced, "S2"),
	))

	current, ok := st.Conversation("p1")
	require.True(t, ok)
	assert.Equal(t, ledger.ConversationBound, current.State, "a refusal of a session the chat moved off is late")
	assert.Equal(t, "S1", current.SessionID, "returning to an earlier session rebinds it")
	got := st.Conversations()
	require.Len(t, got, 2, "a session is listed once, where it was first bound")
	assert.Equal(t, ledger.ConversationUnbound, got[1].State, "the session the chat moved off is no longer bound")

	clone := st.Clone()
	st.Apply(lostS1(t, "p1"))
	again, _ := clone.Conversation("p1")
	assert.Equal(t, ledger.ConversationBound, again.State, "a clone must not see rows applied to its original")
}
