package ledger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func scoped[B any](t *testing.T, kind ledger.Kind, turnID, scope string, body B) ledger.Row {
	t.Helper()
	r, err := ledger.NewRow(kind, turnID, scope, body)
	require.NoError(t, err)
	return r
}

func toolCall(t *testing.T, scope, toolID, phase, name string) ledger.Row {
	t.Helper()
	return scoped(t, ledger.KindToolCall, "T1", scope, ledger.ToolCall{ToolID: toolID, Phase: phase, Name: name})
}

func TestState_KeepsWhatARetriedToolPhaseIsRecognisedBy(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		turnStart(t, "T1"),
		toolCall(t, "", "tool-1", ledger.ToolPre, "Bash"),
		toolCall(t, "A", "tool-1", ledger.ToolPre, "Bash"),
		toolCall(t, "", "tool-1", ledger.ToolPost, ""),
	))

	assert.True(t, st.HasToolPhase("", "tool-1", ledger.ToolPre))
	assert.True(t, st.HasToolPhase("", "tool-1", ledger.ToolPost))
	assert.False(t, st.HasToolPhase("A", "tool-1", ledger.ToolPost), "a scope's call is its own")
	turn, ok := st.ToolTurn("A", "tool-1")
	require.True(t, ok)
	assert.Equal(t, "T1", turn)
}

func TestState_OpenToolIsTheNewestUnfinishedCallOfItsName(t *testing.T) {
	st := ledger.Fold("c1", sequenced(t,
		turnStart(t, "T1"),
		toolCall(t, "", "old", ledger.ToolPre, "Bash"),
		toolCall(t, "", "new", ledger.ToolPre, "Bash"),
		toolCall(t, "", "other", ledger.ToolPre, "Edit"),
		toolCall(t, "", "new", ledger.ToolFail, "Bash"),
	))

	id, ok := st.OpenTool("", "Bash")
	require.True(t, ok)
	assert.Equal(t, "old", id, "a failed call is closed")
	_, ok = st.OpenTool("A", "Bash")
	assert.False(t, ok)
}

func TestState_AToolSpawningAScopeOpensItOnce(t *testing.T) {
	spawn := scoped(t, ledger.KindToolCall, "T1", "", ledger.ToolCall{
		ToolID: "spawn", Phase: ledger.ToolPost, Name: "spawnAgent", SpawnsScope: "child",
	})
	st := ledger.Fold("c1", sequenced(t,
		turnStart(t, "T1"), spawn,
		scoped(t, ledger.KindSubagent, "T1", "child", ledger.Subagent{Phase: ledger.SubagentStop}),
		spawn,
	))

	assert.False(t, st.ScopeOpen("child"), "a stopped scope is never reopened")
	turn, ok := st.ScopeTurn("child")
	require.True(t, ok)
	assert.Equal(t, "T1", turn)
}

func TestCheck_AToolCallNeedsAnIDAndAPhase(t *testing.T) {
	require.NoError(t, ledger.Check(ledger.State{}, toolCall(t, "", "t", ledger.ToolPre, "Bash")))
	require.ErrorIs(t, ledger.Check(ledger.State{}, toolCall(t, "", "", ledger.ToolPre, "Bash")), ledger.ErrMalformedRow)
	require.ErrorIs(t, ledger.Check(ledger.State{}, toolCall(t, "", "t", "later", "Bash")), ledger.ErrMalformedRow)
}
