package ledger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func subagent(t *testing.T, scope, phase string) ledger.Row {
	t.Helper()
	return scoped(t, ledger.KindSubagent, "T1", scope, ledger.Subagent{Phase: phase, Type: "explorer"})
}

func TestState_KeepsEachScopesPhases(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t, turnStart(t, "T1"), subagent(t, "A", ledger.SubagentStart)))

	assert.True(t, st.ScopeOpen("A"))
	assert.True(t, st.HasScopePhase("A", ledger.SubagentStart))
	assert.False(t, st.HasScopePhase("A", ledger.SubagentStop))
	assert.False(t, st.HasScopePhase("B", ledger.SubagentStart))

	st.Apply(ledger.Row{
		Seq: 3, Kind: ledger.KindSubagent, TurnID: "T1", Scope: "A",
		Body: subagent(t, "A", ledger.SubagentStop).Body,
	})
	assert.False(t, st.ScopeOpen("A"))
	assert.True(t, st.HasScopePhase("A", ledger.SubagentStop))
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

func TestState_ARunnersExitStopsEveryScope(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		turnStart(t, "T1"), subagent(t, "A", ledger.SubagentStart),
		row(t, ledger.KindRunnerExited, "", ledger.RunnerExited{RunnerID: "run-1", Reason: "exited"}),
	))

	assert.False(t, st.ScopeOpen("A"))
}

func TestCheck_ASubagentRowNeedsItsScope(t *testing.T) {
	require.NoError(t, ledger.Check(ledger.State{}, subagent(t, "A", ledger.SubagentStart)))
	require.ErrorIs(t, ledger.Check(ledger.State{}, subagent(t, "", ledger.SubagentStart)), ledger.ErrMalformedRow)
	require.ErrorIs(t, ledger.Check(ledger.State{}, subagent(t, "A", "paused")), ledger.ErrMalformedRow)
	plan := row(t, ledger.KindPlan, "T1", ledger.Plan{Op: ledger.PlanReplace, Items: []ledger.PlanItem{{Text: "x"}}})
	require.NoError(t, ledger.Check(ledger.State{}, plan))
	require.ErrorIs(t, ledger.Check(ledger.State{}, row(t, ledger.KindPlan, "T1", ledger.Plan{})), ledger.ErrMalformedRow)
}
