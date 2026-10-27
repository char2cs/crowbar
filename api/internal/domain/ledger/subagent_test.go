package ledger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func subagent(t *testing.T, scope, phase string) ledger.Row {
	t.Helper()
	return scoped(t, ledger.KindSubagent, "T1", scope, ledger.SubagentBody{Phase: phase, Type: "explorer"})
}

// spawned is the subagent row of a spawning call: starting under toolID, or
// bound to scope when one is named.
func spawned(t *testing.T, scope, phase, toolID string) ledger.Row {
	t.Helper()
	return scoped(t, ledger.KindSubagent, "T1", scope, ledger.SubagentBody{Phase: phase, SpawnToolID: toolID})
}

func TestSubagentStatus_Next(t *testing.T) {
	allowed := map[ledger.SubagentStatus][]ledger.SubagentStatus{
		ledger.SubagentStarting: {ledger.SubagentRunning, ledger.SubagentFailed, ledger.SubagentAbandoned, ledger.SubagentInterrupted},
		ledger.SubagentRunning:  {ledger.SubagentDone, ledger.SubagentFailed, ledger.SubagentInterrupted},
	}
	all := []ledger.SubagentStatus{
		ledger.SubagentStarting, ledger.SubagentRunning, ledger.SubagentDone, ledger.SubagentFailed,
		ledger.SubagentInterrupted, ledger.SubagentAbandoned,
	}
	for _, from := range all {
		for _, to := range all {
			got, ok := from.Next(to)
			want := false
			for _, a := range allowed[from] {
				want = want || a == to
			}
			assert.Equal(t, want, ok, "%s → %s", from, to)
			if !ok {
				assert.Equal(t, from, got, "a refused move stays where it was")
			}
		}
	}
}

func TestFold_ASubagentOutlivesItsParentTurnAndKeepsTheChatBusy(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		turnStart(t, "T1"),
		spawned(t, "", ledger.SubagentStart, "tool-1"),
		spawned(t, "A", ledger.SubagentBound, "tool-1"),
		ended(t, "T1"),
	))

	require.True(t, st.Busy(), "a running subagent keeps the chat busy past its turn")
	a, ok := st.SubagentOf("A")
	require.True(t, ok)
	assert.Equal(t, "tool-1", a.SpawnToolID)
	assert.Equal(t, "T1", a.TurnID)
	assert.False(t, st.ScopeWaits("A"), "what it asks after the end is no longer waited on")

	stop := scoped(t, ledger.KindSubagent, "T1", "A", ledger.SubagentBody{
		Phase: ledger.SubagentStop, Result: "found it", Usage: &ledger.SubagentUsage{Tokens: 9, ToolUses: 2},
	})
	stop.Seq = 5
	st.Apply(stop)
	assert.False(t, st.Busy())
	a, _ = st.SubagentOf("A")
	assert.Equal(t, ledger.SubagentDone, a.Status)
	assert.Equal(t, "found it", a.Result)
	assert.Equal(t, ledger.SubagentUsage{Tokens: 9, ToolUses: 2}, a.Usage)
}

func TestFold_AnUnboundSubagentIsAbandonedWhenItsTurnEnds(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		turnStart(t, "T1"), spawned(t, "", ledger.SubagentStart, "tool-1"),
	))
	require.True(t, st.Busy())
	starting, ok := st.SpawnedBy("tool-1")
	require.True(t, ok)
	assert.Equal(t, ledger.SubagentStarting, starting.Status)

	st.Apply(ledger.Row{Seq: 3, Kind: ledger.KindTurnEnded, TurnID: "T1", Body: ended(t, "T1").Body})

	assert.False(t, st.Busy(), "nothing bound it, so nothing keeps the chat busy")
	gone, _ := st.SpawnedBy("tool-1")
	assert.Equal(t, ledger.SubagentAbandoned, gone.Status)
}

func TestFold_StopInterruptsEveryLiveSubagent(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		turnStart(t, "T1"),
		spawned(t, "", ledger.SubagentStart, "tool-1"),
		spawned(t, "", ledger.SubagentStart, "tool-2"),
		spawned(t, "A", ledger.SubagentBound, "tool-1"),
	))

	rows, err := ledger.Stop(st)
	require.NoError(t, err)
	for i, r := range rows {
		r.Seq = st.Head() + int64(i) + 1
		st.Apply(r)
	}

	assert.False(t, st.Busy())
	a, _ := st.SubagentOf("A")
	unbound, _ := st.SpawnedBy("tool-2")
	assert.Equal(t, ledger.SubagentInterrupted, a.Status)
	assert.Equal(t, ledger.SubagentInterrupted, unbound.Status)
}

func TestFold_ARunnersExitInterruptsItsSubagents(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		turnStart(t, "T1"), subagent(t, "A", ledger.SubagentStart), ended(t, "T1"),
		row(t, ledger.KindRunnerExited, "", ledger.RunnerExited{RunnerID: "run-1", Reason: "exited"}),
	))

	assert.False(t, st.Busy())
	assert.False(t, st.ScopeWaits("A"))
	assert.True(t, st.ScopeOpen("A"), "its late reports still reach it")
}

// A start that names its scope (a subagent announcing itself) runs at once;
// the call that spawned it binds to it later, and the record keeps the call.
func TestFold_ASubagentThatAnnouncesItselfFirstTakesItsCallOnTheBind(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		turnStart(t, "T1"),
		scoped(t, ledger.KindSubagent, "T1", "", ledger.SubagentBody{Phase: ledger.SubagentStart, SpawnToolID: "tool-1", Title: "Read a.txt"}),
		subagent(t, "A", ledger.SubagentStart),
		spawned(t, "A", ledger.SubagentBound, "tool-1"),
	))

	all := st.Subagents()
	require.Len(t, all, 1, "the call and the announcement are one subagent")
	assert.Equal(t, ledger.Subagent{
		Scope: "A", SpawnToolID: "tool-1", Type: "explorer", Title: "Read a.txt", Status: ledger.SubagentRunning,
		TurnID: "T1", StartedAt: all[0].StartedAt, Seq: 2,
	}, stripped(all[0]))
}

// stripped is a with only its exported fields, for comparison.
func stripped(a ledger.Subagent) ledger.Subagent {
	return ledger.Subagent{
		Scope: a.Scope, SpawnToolID: a.SpawnToolID, Type: a.Type, Title: a.Title, Result: a.Result, Status: a.Status,
		TurnID: a.TurnID, StartedAt: a.StartedAt, EndedAt: a.EndedAt, Usage: a.Usage, Seq: a.Seq,
	}
}

func TestState_AScopeATurnEndOutlivedIsNotWaitedOn(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t, turnStart(t, "T1"), subagent(t, "A", ledger.SubagentStart), ended(t, "T1")))

	assert.False(t, st.ScopeWaits("A"), "a turn's end leaves nothing it ran to wait on")
	assert.True(t, st.ScopeOpen("A"), "while its own reports still reach it")

	st.Apply(ledger.Row{
		Seq: 4, Kind: ledger.KindSubagent, TurnID: "T1", Scope: "B",
		Body: subagent(t, "B", ledger.SubagentStart).Body,
	})
	assert.True(t, st.ScopeWaits("B"), "one opened after the end outlives it, as a delegated agent does")
}

func TestState_AStoppedScopeIsNeverReopened(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		turnStart(t, "T1"), subagent(t, "A", ledger.SubagentStart), subagent(t, "A", ledger.SubagentStop),
		subagent(t, "A", ledger.SubagentStart),
	))

	assert.False(t, st.ScopeOpen("A"))
	a, _ := st.SubagentOf("A")
	assert.Equal(t, ledger.SubagentDone, a.Status)
}

func TestCheck_ASubagentRowNeedsItsScopeOrItsCall(t *testing.T) {
	require.NoError(t, ledger.Check(ledger.State{}, subagent(t, "A", ledger.SubagentStart)))
	require.NoError(t, ledger.Check(ledger.State{}, spawned(t, "", ledger.SubagentStart, "tool-1")))
	require.ErrorIs(t, ledger.Check(ledger.State{}, subagent(t, "", ledger.SubagentStart)), ledger.ErrMalformedRow)
	require.ErrorIs(t, ledger.Check(ledger.State{}, subagent(t, "A", "paused")), ledger.ErrMalformedRow)
	require.ErrorIs(t, ledger.Check(ledger.State{}, spawned(t, "", ledger.SubagentBound, "tool-1")), ledger.ErrMalformedRow,
		"a bind names the scope it binds")
	plan := row(t, ledger.KindPlan, "T1", ledger.Plan{Op: ledger.PlanReplace, Items: []ledger.PlanItem{{Text: "x"}}})
	require.NoError(t, ledger.Check(ledger.State{}, plan))
	require.ErrorIs(t, ledger.Check(ledger.State{}, row(t, ledger.KindPlan, "T1", ledger.Plan{})), ledger.ErrMalformedRow)
}
