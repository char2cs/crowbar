package chatledger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func TestConvertChat_ToolsKeepTheirBlobRefsAndScopes(t *testing.T) {
	res := convert(t, chatA(), nobody)

	tools := rowsOf(res, ledger.KindToolCall)
	require.Len(t, tools, 4)
	pre, post := body[ledger.ToolCall](t, tools[0]), body[ledger.ToolCall](t, tools[1])
	assert.Equal(t, ledger.ToolCall{
		ToolID: "t-1", Phase: ledger.ToolPre, Name: "Read", Kind: "read",
		Locations: []ledger.ToolLocation{{Path: "/w/a.go", Line: 3}}, InputRef: "req/t-1",
	}, pre)
	assert.Equal(t, ledger.ToolCall{
		ToolID: "t-1", Phase: ledger.ToolPost, Name: "Read", Kind: "read", ResultRef: "res/t-1", Status: "ok", DurationMS: 40,
	}, post)
	assert.Equal(t, turnID("chat-a", "req-1"), tools[0].TurnID, "an open reply's id names its runner's turn")
	assert.Empty(t, tools[0].Scope)
	nested := body[ledger.ToolCall](t, tools[3])
	assert.Equal(t, ledger.ToolFail, nested.Phase)
	assert.Equal(t, "res/t-2", nested.ResultRef)
	assert.Equal(t, "sa-1", tools[2].Scope, "a subagent's call is in its scope")
	assert.Equal(t, turnID("chat-a", "u-2"), tools[2].TurnID, "in the turn that spawned the subagent")
}

func TestConvertChat_AToolNoTurnHoldsIsSkipped(t *testing.T) {
	in := chatA()
	in.Tools[0].TurnID = "msg-gone"

	res := convert(t, in, nobody)

	assert.Contains(t, res.Skipped, "tool t-1: no turn of the chat holds it")
	assert.Len(t, rowsOf(res, ledger.KindToolCall), 2)
}

func TestConvertChat_AnUnendedSubagentIsAbandoned(t *testing.T) {
	res := convert(t, chatA(), nobody)

	st := fold(res)
	assert.Empty(t, st.Subagents(), "a resting chat has no live subagent")
	stops := bodies[ledger.SubagentBody](t, res, ledger.KindSubagent)
	assert.Equal(t, ledger.SubagentBody{Phase: ledger.SubagentStop, Result: "found it"}, stops[1],
		"one that ended is done, its last reply its result")
	assert.Equal(t, ledger.SubagentBody{Phase: ledger.SubagentStop, Status: string(ledger.SubagentInterrupted)}, stops[3],
		"one that never ended stops when its turn does")
	var unended ledger.Row
	for _, r := range rowsOf(res, ledger.KindSubagent) {
		if r.Scope == "sa-2" {
			unended = r
		}
	}
	end := rowsOf(res, ledger.KindTurnEnded)[2]
	assert.Equal(t, end.At, unended.At)
	assert.Less(t, unended.Seq, end.Seq)
}
