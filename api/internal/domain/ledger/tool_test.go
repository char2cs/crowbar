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

	id, ok := st.OpenTool("", "Bash", "")
	require.True(t, ok)
	assert.Equal(t, "old", id, "a failed call is closed")
	_, ok = st.OpenTool("A", "Bash", "")
	assert.False(t, ok)
}

// Of two open calls of one name, an input names the one it was given to; a
// call whose input was never recorded is told apart by its name alone.
func TestState_OpenToolOfAnInputIsTheCallGivenThatInput(t *testing.T) {
	keyed := func(id, input string) ledger.Row {
		return scoped(t, ledger.KindToolCall, "T1", "", ledger.ToolCall{
			ToolID: id, Phase: ledger.ToolPre, Name: "Bash", InputKey: ledger.InputKey([]byte(input)),
		})
	}
	st := ledger.Fold("c1", sequenced(t,
		turnStart(t, "T1"), keyed("ls", `{"command":"ls","timeout":5}`), keyed("pwd", `{"command":"pwd"}`),
		toolCall(t, "", "unkeyed", ledger.ToolPre, "Read"),
	))

	for name, tc := range map[string]struct{ name, input, want string }{
		"the older call's input, reordered": {"Bash", `{ "timeout": 5, "command": "ls" }`, "ls"},
		"the newer call's input":            {"Bash", `{"command":"pwd"}`, "pwd"},
		"no input":                          {"Bash", "", "pwd"},
		"a call of no recorded input":       {"Read", `{"file_path":"a.go"}`, "unkeyed"},
	} {
		t.Run(name, func(t *testing.T) {
			id, ok := st.OpenTool("", tc.name, ledger.InputKey([]byte(tc.input)))
			require.True(t, ok)
			assert.Equal(t, tc.want, id)
		})
	}
	_, ok := st.OpenTool("", "Bash", ledger.InputKey([]byte(`{"command":"rm"}`)))
	assert.False(t, ok, "an input no open call of its name was given binds to none")

	raw := toolCall(t, "", "raw", ledger.ToolPre, "Bash")
	raw.Seq = st.Head() + 1
	st.Apply(raw)
	id, _ := st.OpenTool("", "Bash", ledger.InputKey([]byte(`{"command":"pwd"}`)))
	assert.Equal(t, "pwd", id, "the call given the input wins over a newer one of no recorded input")
	id, _ = st.OpenTool("", "Bash", ledger.InputKey([]byte(`{"command":"rm"}`)))
	assert.Equal(t, "raw", id)
}

func TestInputKey_IsTheSameForEqualJSONAndNoneWithoutIt(t *testing.T) {
	assert.Equal(t, ledger.InputKey([]byte(`{"a":1,"b":[true,null]}`)), ledger.InputKey([]byte("{\"b\": [true, null],\n \"a\": 1}")))
	assert.NotEqual(t, ledger.InputKey([]byte(`{"a":1}`)), ledger.InputKey([]byte(`{"a":"1"}`)))
	assert.NotEqual(t, ledger.InputKey([]byte(`{"n":9007199254740993}`)), ledger.InputKey([]byte(`{"n":9007199254740992}`)),
		"two numbers a float cannot tell apart are still two inputs")
	assert.Empty(t, ledger.InputKey(nil))
	assert.Empty(t, ledger.InputKey([]byte(`{"a":`)), "a truncated input names no call")
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
