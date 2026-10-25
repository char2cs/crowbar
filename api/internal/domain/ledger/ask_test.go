package ledger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func ask(t *testing.T, a ledger.Ask) ledger.Row {
	t.Helper()
	if a.Event == "" {
		a.Event = "permission"
	}
	return row(t, ledger.KindAsk, "T1", a)
}

func TestState_AnAskIsPendingUntilResolved(t *testing.T) {
	rows := sequenced(t,
		turnStart(t, "T1"),
		ask(t, ledger.Ask{AskID: "a1", ToolID: "tool-1", ToolName: "Bash"}),
		ask(t, ledger.Ask{AskID: "a2", PromptID: "p2"}),
	)
	st := ledger.Fold("c1", rows)

	pending := st.PendingAsks()
	require.Len(t, pending, 2)
	assert.Equal(t, []string{"a1", "a2"}, []string{pending[0].AskID, pending[1].AskID}, "oldest first")
	assert.Equal(t, "T1", pending[0].TurnID)
	assert.Equal(t, []string{"a1"}, st.AsksOfTool("", "tool-1"))

	resolved, err := ledger.ResolveAsk(st, "a1", ledger.ResolutionAnswered, []string{"allow"})
	require.NoError(t, err)
	require.Len(t, resolved, 1)
	resolved[0].Seq = 4
	st.Apply(resolved[0])
	assert.Len(t, st.PendingAsks(), 1)

	again, err := ledger.ResolveAsk(st, "a1", ledger.ResolutionAnswered, nil)
	require.NoError(t, err)
	assert.Empty(t, again, "a closed ask is closed once")
}

func TestState_SameAskRecognisesARetriedDelivery(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		turnStart(t, "T1"),
		ask(t, ledger.Ask{AskID: "tool", ToolID: "tool-1", ToolName: "Bash"}),
		ask(t, ledger.Ask{AskID: "prompt", PromptID: "p1", ToolName: "Read"}),
		ask(t, ledger.Ask{AskID: "bare", ToolName: "Edit", Question: "may I?"}),
	))

	for name, tc := range map[string]struct {
		ask  ledger.Ask
		want string
	}{
		"by tool":                 {ledger.Ask{Event: "permission", ToolID: "tool-1"}, "tool"},
		"by prompt id & tool":     {ledger.Ask{Event: "permission", PromptID: "p1", ToolName: "Read"}, "prompt"},
		"by tool name & question": {ledger.Ask{Event: "permission", ToolName: "Edit", Question: "may I?"}, "bare"},
	} {
		t.Run(name, func(t *testing.T) {
			id, ok := st.SameAsk("", tc.ask)
			require.True(t, ok)
			assert.Equal(t, tc.want, id)
		})
	}
	_, ok := st.SameAsk("", ledger.Ask{Event: "elicitation", ToolID: "tool-1"})
	assert.False(t, ok, "another event is another ask")
	_, ok = st.SameAsk("A", ledger.Ask{Event: "permission", ToolID: "tool-1"})
	assert.False(t, ok, "another scope is another ask")
	_, ok = st.SameAsk("", ledger.Ask{Event: "permission", ToolID: "tool-2"})
	assert.False(t, ok)
	_, ok = st.SameAsk("", ledger.Ask{Event: "permission", PromptID: "p1", ToolName: "Write"})
	assert.False(t, ok, "a turn's prompt id names every ask the turn raises: another tool is another ask")
}

func TestState_AsksDoNotOutliveTheirTurnOrTheirRunner(t *testing.T) {
	ends := map[string]ledger.Row{
		"its turn ended":  ended(t, "T1"),
		"its turn failed": row(t, ledger.KindTurnFailed, "T1", ledger.TurnFailed{Reason: "rate_limit"}),
		"its runner exited": row(t, ledger.KindRunnerExited, "",
			ledger.RunnerExited{RunnerID: "run-1", Reason: "exited"}),
	}
	for name, end := range ends {
		t.Run(name, func(t *testing.T) {
			st := ledger.Fold("c1", sequenced(t,
				turnStart(t, "T1"), ask(t, ledger.Ask{AskID: "a1", Answerable: true}), end,
			))
			assert.Empty(t, st.PendingAsks())
		})
	}
}

// An end that declared background work leaves a held ask to that work, the only
// thing that can hold the CLI on it now; a subagent's ask closes with it.
func TestState_AHeldAskOutlivesAnEndThatDeclaredBackgroundWork(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		turnStart(t, "T1"), subagent(t, "A", ledger.SubagentStart),
		ask(t, ledger.Ask{AskID: "held", Answerable: true}),
		ask(t, ledger.Ask{AskID: "unheld"}),
		scoped(t, ledger.KindAsk, "T1", "A", ledger.Ask{AskID: "held-in-A", Event: "permission", Answerable: true}),
		row(t, ledger.KindTurnEnded, "T1", ledger.TurnEnded{Status: "done", RunnerID: "run-1", AsyncWork: new(1)}),
	))
	open := func() []string {
		var ids []string
		for _, a := range st.PendingAsks() {
			ids = append(ids, a.AskID)
		}
		return ids
	}
	assert.Equal(t, []string{"held", "held-in-A"}, open(), "nothing holds the unheld one")

	stop := subagent(t, "A", ledger.SubagentStop)
	stop.Seq = 7
	st.Apply(stop)
	assert.Equal(t, []string{"held"}, open(), "A's ask closes with A")
}

func TestCheck_AnAskNeedsAnIDAndAnEventAndClosesAsAKnownResolution(t *testing.T) {
	require.NoError(t, ledger.Check(ledger.State{}, ask(t, ledger.Ask{AskID: "a"})))
	require.ErrorIs(t, ledger.Check(ledger.State{}, ask(t, ledger.Ask{})), ledger.ErrMalformedRow)
	resolved := row(t, ledger.KindAskResolved, "T1", ledger.AskResolved{AskID: "a", Resolution: "maybe"})
	require.ErrorIs(t, ledger.Check(ledger.State{}, resolved), ledger.ErrMalformedRow)
	for _, how := range []string{
		ledger.ResolutionAnswered, ledger.ResolutionProceeded, ledger.ResolutionRefused, ledger.ResolutionAbandoned,
	} {
		resolved := row(t, ledger.KindAskResolved, "T1", ledger.AskResolved{AskID: "a", Resolution: how})
		require.NoError(t, ledger.Check(ledger.State{}, resolved), how)
	}
}
