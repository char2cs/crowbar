package transcript_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func tool(t *testing.T, scope string, body ledger.ToolCall) ledger.Row {
	t.Helper()
	return mk(t, ledger.KindToolCall, "T1", scope, body)
}

func marker(t *testing.T, turnID, kind, detail string) ledger.Row {
	t.Helper()
	return mk(t, ledger.KindMarker, turnID, "", ledger.Marker{Kind: kind, Detail: detail})
}

func TestProjector_ToolsAttachToTheReplyThatFollowsThem(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.say("", "m1", "looking")
	b.add(tool(t, "", ledger.ToolCall{ToolID: "t1", Phase: ledger.ToolPre, Name: "Read"}))
	b.say("", "m2", "found it")
	b.add(tool(t, "", ledger.ToolCall{ToolID: "t2", Phase: ledger.ToolPre, Name: "Bash"}))

	calls := b.activity().ToolCalls
	require.Len(t, calls, 2)
	assert.Equal(t, "msg-m2", calls[0].TurnID, "a tool is shown with the message that follows it")
	assert.Equal(t, "msg-final-T1", calls[1].TurnID, "with none yet, the reply its turn's end records")
}

func TestProjector_AToolCallMergesItsPhases(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(tool(t, "", ledger.ToolCall{
		ToolID: "t1", Phase: ledger.ToolPre, Name: "Edit", Kind: "edit", Target: "a.go", InputRef: "in-1",
		Locations: []ledger.ToolLocation{{Path: "a.go", Line: 3}},
	}))
	assert.Equal(t, domain.ToolStatusRunning, b.activity().ToolCalls[0].Status)

	b.add(tool(t, "", ledger.ToolCall{
		ToolID: "t1", Phase: ledger.ToolFail, Status: domain.ToolStatusError, Error: "denied",
		DurationMS: 12, ResultRef: "out-1", Diff: "+y",
	}))
	b.add(tool(t, "", ledger.ToolCall{ToolID: "t1", Phase: ledger.ToolPost, Status: domain.ToolStatusOK}))

	got := b.activity().ToolCalls
	require.Len(t, got, 1)
	call := got[0]
	assert.Equal(t, "t1", call.ID)
	assert.Equal(t, chatID, call.ChatID)
	assert.Equal(t, "Edit", call.Name)
	assert.Equal(t, "edit", call.Kind)
	assert.Equal(t, "a.go", call.Target)
	assert.Equal(t, "in-1", call.RequestRef)
	assert.Equal(t, "out-1", call.ResultRef)
	assert.Equal(t, "+y", call.Diff)
	assert.Equal(t, []domain.ActivityToolLocation{{Path: "a.go", Line: 3}}, call.Locations)
	assert.Equal(t, domain.ToolStatusError, call.Status, "its first close is its outcome")
	assert.Equal(t, "denied", call.Error)
	assert.Equal(t, 12, call.DurationMS)
	require.NotNil(t, call.EndedAt)
}

// A call keeps the title it started with, and takes its exit code from its end.
func TestProjector_AToolCallKeepsItsStartsTitleAndItsEndsExitCode(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(tool(t, "", ledger.ToolCall{
		ToolID: "t1", Phase: ledger.ToolPre, Name: "shout", Kind: ledger.ToolKindMCP, Title: "Shout it",
		MCPServer: "echo", MCPTool: "shout",
	}))
	assert.Nil(t, b.activity().ToolCalls[0].ExitCode, "a running call has no exit code")

	b.add(tool(t, "", ledger.ToolCall{ToolID: "t1", Phase: ledger.ToolFail, Title: "retitled", ExitCode: new(2)}))

	call := b.activity().ToolCalls[0]
	assert.Equal(t, "Shout it", call.Title)
	assert.Equal(t, "echo", call.MCPServer)
	assert.Equal(t, "shout", call.MCPTool)
	require.NotNil(t, call.ExitCode)
	assert.Equal(t, 2, *call.ExitCode)
}

// A call whose provider reports no duration takes the one its rows observed,
// but only from a start that was seen: none is invented.
func TestProjector_AnUnreportedDurationIsTheOneItsRowsObserved(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(tool(t, "", ledger.ToolCall{ToolID: "seen", Phase: ledger.ToolPre, Name: "x"}))
	b.add(tool(t, "", ledger.ToolCall{ToolID: "seen", Phase: ledger.ToolPost}))
	b.add(tool(t, "", ledger.ToolCall{ToolID: "unseen", Phase: ledger.ToolPost}))

	got := b.activity().ToolCalls
	require.Len(t, got, 2)
	assert.Equal(t, 1000, got[0].DurationMS, "a commit a second")
	assert.Zero(t, got[1].DurationMS)
}

func TestProjector_AToolOpenWhenItsTurnEndedReadsAbandoned(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(tool(t, "", ledger.ToolCall{ToolID: "t1", Phase: ledger.ToolPre, Name: "Bash"}))
	b.end()

	call := b.activity().ToolCalls[0]
	assert.Equal(t, domain.ToolStatusAbandoned, call.Status)
	require.NotNil(t, call.EndedAt)
}

func TestProjector_AToolOpenWhenItsRunnerExitedReadsAbandoned(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(tool(t, "", ledger.ToolCall{ToolID: "t1", Phase: ledger.ToolPre, Name: "Bash"}))
	b.add(mk(t, ledger.KindRunnerExited, "", "", ledger.RunnerExited{RunnerID: "run-1", Reason: "exited"}))

	assert.Equal(t, domain.ToolStatusAbandoned, b.activity().ToolCalls[0].Status)
}

func TestProjector_AScopesToolCallsAreItsSubagents(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(tool(t, "", ledger.ToolCall{ToolID: "spawn", Phase: ledger.ToolPost, Name: "Agent", SpawnsScope: "A"}))
	b.add(tool(t, "A", ledger.ToolCall{ToolID: "t1", Phase: ledger.ToolPre, Name: "Read"}))
	b.add(tool(t, "B", ledger.ToolCall{ToolID: "t1", Phase: ledger.ToolPre, Name: "Bash"}))

	calls := b.activity().ToolCalls
	require.Len(t, calls, 3, "one id in two scopes is two calls")
	assert.Empty(t, calls[0].SubagentID)
	assert.Equal(t, "A", calls[1].SubagentID)
	assert.Empty(t, calls[1].TurnID, "a nested call has no message of the chat's")
	assert.Equal(t, "B", calls[2].SubagentID)
}

func TestProjector_ASubagentIsOpenedByItsStartOrItsSpawn(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(mk(t, ledger.KindSubagent, "T1", "A", ledger.Subagent{Phase: ledger.SubagentStart, Type: "explorer"}))
	b.add(tool(t, "", ledger.ToolCall{ToolID: "spawn", Phase: ledger.ToolPost, SpawnsScope: "B"}))
	b.add(mk(t, ledger.KindSubagent, "T1", "C", ledger.Subagent{Phase: ledger.SubagentStop}))
	b.say("A", "a1", "found it")
	b.add(mk(t, ledger.KindSubagent, "T1", "A", ledger.Subagent{Phase: ledger.SubagentStop}))

	subs := b.activity().Subagents
	require.Len(t, subs, 2, "a scope only its stop names was never opened")
	assert.Equal(t, "A", subs[0].ID)
	assert.Equal(t, "explorer", subs[0].AgentType)
	require.NotNil(t, subs[0].EndedAt)
	require.Len(t, subs[0].Messages, 1)
	assert.Equal(t, "found it", subs[0].Messages[0].Text)
	assert.Equal(t, "B", subs[1].ID)
	assert.Nil(t, subs[1].EndedAt)
}

func TestProjector_ASubagentOpenWhenItsTurnEndedReadsStopped(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(mk(t, ledger.KindSubagent, "T1", "A", ledger.Subagent{Phase: ledger.SubagentStart}))
	b.end()

	sub := b.activity().Subagents[0]
	require.NotNil(t, sub.EndedAt, "a stop that never came is its turn's end")
}

func TestProjector_ASubagentOpenWhenItsRunnerExitedReadsStopped(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.end()
	b.add(tool(t, "", ledger.ToolCall{ToolID: "spawn", Phase: ledger.ToolPost, SpawnsScope: "A"}))
	assert.Nil(t, b.activity().Subagents[0].EndedAt, "one opened after its turn ended outlives that end")

	b.add(mk(t, ledger.KindRunnerExited, "", "", ledger.RunnerExited{RunnerID: "run-1", Reason: "exited"}))

	require.NotNil(t, b.activity().Subagents[0].EndedAt)
}

// A turn that ends declaring background work leaves its subagents running:
// each is over at its own stop or its runner's exit, and a tool of one is
// abandoned only when that subagent ends with it open.
func TestProjector_ABackgroundSubagentRunsPastItsTurnsEnd(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(
		mk(t, ledger.KindSubagent, "T1", "A", ledger.Subagent{Phase: ledger.SubagentStart}),
		mk(t, ledger.KindSubagent, "T1", "B", ledger.Subagent{Phase: ledger.SubagentStart}),
		tool(t, "A", ledger.ToolCall{ToolID: "read", Phase: ledger.ToolPre, Name: "Read"}),
		mk(t, ledger.KindTurnEnded, "T1", "", ledger.TurnEnded{Status: "done", RunnerID: "run-1", AsyncWork: new(2)}),
		tool(t, "A", ledger.ToolCall{ToolID: "bash", Phase: ledger.ToolPre, Name: "Bash"}),
		tool(t, "B", ledger.ToolCall{ToolID: "grep", Phase: ledger.ToolPre, Name: "Grep"}),
	)
	status := func() map[string]string {
		out := map[string]string{}
		for _, s := range b.activity().Subagents {
			out[s.ID] = "running"
			if s.EndedAt != nil {
				out[s.ID] = "stopped"
			}
		}
		for _, c := range b.activity().ToolCalls {
			out[c.Name] = c.Status
		}
		return out
	}
	running := domain.ToolStatusRunning
	assert.Equal(t, map[string]string{"A": "running", "B": "running", "Read": running, "Bash": running, "Grep": running}, status())

	b.add(mk(t, ledger.KindSubagent, "T1", "A", ledger.Subagent{Phase: ledger.SubagentStop}))
	abandoned := domain.ToolStatusAbandoned
	assert.Equal(t, map[string]string{"A": "stopped", "B": "running", "Read": abandoned, "Bash": abandoned, "Grep": running}, status())

	b.add(mk(t, ledger.KindRunnerExited, "", "", ledger.RunnerExited{RunnerID: "run-1", Reason: "exited"}))
	assert.Equal(t, map[string]string{"A": "stopped", "B": "stopped", "Read": abandoned, "Bash": abandoned, "Grep": abandoned}, status())
}

func TestProjector_InterruptionsAreTheMarkersAClientDraws(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(
		marker(t, "T1", ledger.MarkerModelChanged, "m2"),
		marker(t, "", ledger.MarkerDeadLetter, "refused"),
		marker(t, "", ledger.MarkerSessionChanged, ""),
		marker(t, "T1", ledger.MarkerCompactionStarted, "auto"),
		marker(t, "T1", ledger.MarkerCompactionStarted, "again"),
	)
	open := b.activity().Interruptions
	require.Len(t, open, 2)
	assert.Equal(t, ledger.MarkerModelChanged, open[0].Kind)
	assert.Equal(t, open[0].At, *open[0].ResolvedAt, "what is only reported is over as it is said")
	assert.Equal(t, "compaction", open[1].Kind)
	assert.Equal(t, "auto", open[1].Detail)
	assert.Nil(t, open[1].ResolvedAt, "a compaction runs until its end")

	b.add(marker(t, "T1", ledger.MarkerCompactionEnded, ""), marker(t, "T1", ledger.MarkerCompactionEnded, ""))

	require.NotNil(t, b.activity().Interruptions[1].ResolvedAt)
}

func TestProjector_AnUnendedCompactionIsOverWithItsTurn(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(marker(t, "T1", ledger.MarkerCompactionStarted, ""))
	b.end()
	b.add(marker(t, "T1", ledger.MarkerStopped, ""))

	ints := b.activity().Interruptions
	require.Len(t, ints, 2)
	require.NotNil(t, ints[0].ResolvedAt)
	require.NotNil(t, ints[1].ResolvedAt)
}

func TestProjector_AnAskIsAnInterruptionUntilItCloses(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(mk(t, ledger.KindAsk, "T1", "", ledger.Ask{AskID: "choice-1", Event: "permission", Detail: "Bash"}))

	ints := b.activity().Interruptions
	require.Len(t, ints, 1)
	assert.Equal(t, "interrupt-1", ints[0].ID)
	assert.Equal(t, "permission", ints[0].Kind)
	assert.Nil(t, ints[0].ResolvedAt)

	b.end()
	require.NotNil(t, b.activity().Interruptions[0].ResolvedAt)
}
