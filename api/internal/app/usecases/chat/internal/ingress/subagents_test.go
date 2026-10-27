package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func spawnCall(kind, scope string) engineagents.CanonicalEvent {
	ev := toolEvent(kind, "call-1", "Agent")
	ev.Tool.Kind, ev.Tool.SpawnsScope, ev.Tool.Title = ledger.ToolKindSubagent, scope, "Read a.txt"
	return ev
}

// A spawning call starts its subagent; the call's end binds it, and it keeps
// the chat busy past the turn that spawned it.
func TestMap_ASpawningCallStartsItsSubagentAndItsEndBindsIt(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("delegate")

	rows, _ := c.mapped(spawnCall(engineagents.HookToolPre, ""), hooksSource())
	require.Equal(t, []ledger.Kind{ledger.KindToolCall, ledger.KindSubagent}, kinds(rows))
	assert.Equal(t, ledger.SubagentBody{Phase: ledger.SubagentStart, SpawnToolID: "call-1", Title: "Read a.txt"},
		bodyOf[ledger.SubagentBody](t, rows[1]))

	rows, _ = c.mapped(spawnCall(engineagents.HookToolPost, "A"), hooksSource())
	require.Equal(t, []ledger.Kind{ledger.KindToolCall, ledger.KindSubagent}, kinds(rows))
	assert.Equal(t, "A", rows[1].Scope)
	assert.Equal(t, "A", boundScope(rows))
	assert.Equal(t, "A", bodyOf[ledger.ToolCall](t, rows[0]).SpawnsScope)

	again, _ := c.mapped(spawnCall(engineagents.HookToolPost, "A"), hooksSource())
	assert.Empty(t, again, "a retried end binds nothing twice")
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop}, hooksSource())
	assert.True(t, c.st.Busy(), "the subagent runs past its turn")
	a, _ := c.st.SubagentOf("A")
	assert.Equal(t, turnID, a.TurnID)

	rows, _ = c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookSubagentPost, Subagent: &engineagents.SubagentEvent{
		ID: "A", Result: "alpha", Usage: &engineagents.SubagentUsage{Tokens: 7},
	}}, hooksSource())
	assert.Equal(t, ledger.SubagentBody{Phase: ledger.SubagentStop, Result: "alpha", Usage: &ledger.SubagentUsage{Tokens: 7}},
		bodyOf[ledger.SubagentBody](t, rows[0]))
	assert.False(t, c.st.Busy())
}

// A call that only waits on or closes a subagent addresses it and binds nothing.
func TestMap_ACallAddressingASubagentBindsNothing(t *testing.T) {
	c := liveChatRows(t)
	c.typed("delegate")
	wait := toolEvent(engineagents.HookToolPost, "call-2", "wait")
	wait.Tool.Kind, wait.Tool.SpawnsScope, wait.Tool.AddressesScope = "meta", "A", "A"

	rows, _ := c.mapped(wait, hooksSource())

	require.Equal(t, []ledger.Kind{ledger.KindToolCall}, kinds(rows))
	call := bodyOf[ledger.ToolCall](t, rows[0])
	assert.Equal(t, "A", call.AddressesScope)
	assert.Empty(t, call.SpawnsScope)
}

func TestMap_ASpawnThatFailsBeforeItsBindFailsItsSubagent(t *testing.T) {
	c := liveChatRows(t)
	c.typed("delegate")
	c.mapped(spawnCall(engineagents.HookToolPre, ""), hooksSource())

	rows, _ := c.mapped(spawnCall(engineagents.HookToolFail, ""), hooksSource())

	require.Equal(t, []ledger.Kind{ledger.KindToolCall, ledger.KindSubagent}, kinds(rows))
	gone, _ := c.st.SpawnedBy("call-1")
	assert.Equal(t, ledger.SubagentFailed, gone.Status)
}

// A stop of a subagent nothing started and that names no type is the CLI's own.
func TestMap_AStopOfAnUntypedUnknownSubagentIsInternal(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")

	_, _, err := Map(c.st, engineagents.CanonicalEvent{
		Kind: engineagents.HookSubagentPost, Subagent: &engineagents.SubagentEvent{ID: "housekeeping"},
	}, hooksSource(), BlobRefs{})
	require.ErrorIs(t, err, errInternalSubagent)

	rows, _ := c.mapped(engineagents.CanonicalEvent{
		Kind: engineagents.HookSubagentPost, Subagent: &engineagents.SubagentEvent{ID: "B", AgentType: "explorer"},
	}, hooksSource())
	assert.Equal(t, []ledger.Kind{ledger.KindSubagent, ledger.KindSubagent}, kinds(rows), "a typed one is started and stopped")
}

func TestMap_AHandBackOpensAProviderTurnWithNoUserMessage(t *testing.T) {
	c := liveChatRows(t)
	c.typed("delegate")
	c.mapped(engineagents.CanonicalEvent{
		Kind: engineagents.HookSubagentPre, Subagent: &engineagents.SubagentEvent{ID: "A", AgentType: "explorer"},
	}, hooksSource())
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop}, hooksSource())

	rows, _ := c.mapped(engineagents.CanonicalEvent{
		Kind: engineagents.HookSubagentResult, TurnID: "pt-2", Subagent: &engineagents.SubagentEvent{ID: "A", Result: "alpha"},
	}, hooksSource())

	require.Equal(t, []ledger.Kind{ledger.KindSubagent, ledger.KindTurnStarted}, kinds(rows))
	assert.Equal(t, ledger.SubagentResult, bodyOf[ledger.SubagentBody](t, rows[0]).Phase)
	assert.Equal(t, "pt-2", bodyOf[ledger.TurnStarted](t, rows[1]).ProviderTurnID)
	turn, running := c.st.RunningTurn()
	require.True(t, running, "the parent answers the hand-back in a turn of its own")
	assert.Equal(t, rows[1].TurnID, turn.ID)
	a, _ := c.st.SubagentOf("A")
	assert.Equal(t, ledger.SubagentDone, a.Status)

	again, _ := c.mapped(engineagents.CanonicalEvent{
		Kind: engineagents.HookSubagentResult, TurnID: "pt-2", Subagent: &engineagents.SubagentEvent{ID: "A", Result: "alpha"},
	}, hooksSource())
	assert.Empty(t, again, "a retried hand-back records nothing twice")
}

func TestMap_APlanStepIsAnUpsertInItsScope(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("go")

	rows, _ := c.mapped(engineagents.CanonicalEvent{
		Kind: engineagents.HookPlanStep, Step: &engineagents.PlanStepEvent{StepID: "1", Text: "read", Status: "active"},
	}, hooksSource())

	require.Len(t, rows, 1)
	assert.Equal(t, turnID, rows[0].TurnID)
	assert.Equal(t, ledger.Plan{Op: ledger.PlanUpsert, Items: []ledger.PlanItem{{ID: "1", Text: "read", Status: "active"}}},
		bodyOf[ledger.Plan](t, rows[0]))
	none, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookPlanStep, Step: &engineagents.PlanStepEvent{}}, hooksSource())
	assert.Empty(t, none, "a step names its id")
}

// A hand-back of a subagent the ledger never saw start still records it.
func TestMap_AHandBackOfAnUnknownSubagentStartsIt(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop}, hooksSource())

	rows, _ := c.mapped(engineagents.CanonicalEvent{
		Kind: engineagents.HookSubagentResult, TurnID: "pt-2", Subagent: &engineagents.SubagentEvent{ID: "Z", Result: "late"},
	}, hooksSource())

	require.Equal(t, []ledger.Kind{ledger.KindSubagent, ledger.KindSubagent, ledger.KindTurnStarted}, kinds(rows))
	z, _ := c.st.SubagentOf("Z")
	assert.Equal(t, "late", z.Result)
}
