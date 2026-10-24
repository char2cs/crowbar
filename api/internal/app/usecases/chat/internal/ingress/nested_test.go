package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// nestedChat is a chat whose agent opened the conversation "child" in turn turnID.
func nestedChat(t *testing.T) (*chatRows, string) {
	t.Helper()
	c := liveChatRows(t)
	turnID := c.typed("delegate")
	spawn := toolEvent(engineagents.HookToolPost, "spawn-1", "spawn")
	spawn.Tool.NestedSessionID = "child"
	c.mapped(spawn, hooksSource())
	require.True(t, c.st.ScopeOpen("child"))
	return c, turnID
}

func nestedSource() Source {
	s := hooksSource()
	s.Scope = "child"
	return s
}

func TestMap_ANestedConversationsFactsAreItsScopes(t *testing.T) {
	c, _ := nestedChat(t)

	rows, _ := c.mapped(toolEvent(engineagents.HookToolPre, "ct1", "Bash"), nestedSource())
	require.Len(t, rows, 1)
	assert.Equal(t, "child", rows[0].Scope)

	rows, fx := c.mapped(permission(engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"}), nestedSource())
	require.Equal(t, []ledger.Kind{ledger.KindAsk}, kinds(rows), "an open scope's ask is waited on")
	assert.Equal(t, "child", rows[0].Scope)
	assert.NotEmpty(t, fx.HoldAsk)

	none, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt}, nestedSource())
	assert.Empty(t, none, "its parent talking to it opens no turn")
}

func TestMap_ANestedConversationsEndIsItsReplyAndItsStop(t *testing.T) {
	c, turnID := nestedChat(t)

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, Message: "done"}, nestedSource())

	require.Equal(t, []ledger.Kind{ledger.KindAssistantText, ledger.KindSubagent}, kinds(rows))
	assert.Equal(t, turnID, rows[0].TurnID)
	assert.Equal(t, ledger.ReplyID("child"), bodyOf[ledger.AssistantText](t, rows[0]).MessageID)
	assert.Equal(t, ledger.SubagentStop, bodyOf[ledger.Subagent](t, rows[1]).Phase)
	_, running := c.st.RunningTurn()
	assert.True(t, running, "the chat's own turn runs on")

	rows, _ = c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnFailed}, nestedSource())
	assert.Empty(t, rows, "it stops once")
}

// A nested conversation whose opening the ledger no longer holds still has
// its end recorded, in the turn its runner ran.
func TestMap_ANestedEndOfAnUnknownScopeIsStillRecorded(t *testing.T) {
	c := liveChatRows(t)
	turnID := c.typed("delegate")

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, Message: "done"}, nestedSource())

	require.Equal(t, []ledger.Kind{ledger.KindAssistantText, ledger.KindSubagent}, kinds(rows))
	for _, r := range rows {
		assert.Equal(t, turnID, r.TurnID)
		assert.Equal(t, "child", r.Scope)
	}
}

// A conversation a turn end outlived still blocks on what it asks.
func TestMap_ANestedAskAfterTheTurnClosedIsHeldOpen(t *testing.T) {
	c, _ := nestedChat(t)
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, Message: "delegated"}, hooksSource())

	rows, fx := c.mapped(permission(engineagents.ChoicePrompt{Kind: "tool_permission", ToolName: "Bash"}), nestedSource())

	require.Equal(t, []ledger.Kind{ledger.KindAsk}, kinds(rows))
	assert.True(t, bodyOf[ledger.Ask](t, rows[0]).Answerable)
	assert.NotEmpty(t, fx.HoldAsk)
}

func TestMap_ANestedEventItDoesNotRecordWritesNothing(t *testing.T) {
	c, _ := nestedChat(t)

	rows, fx := c.mapped(delta(engineagents.HookMessageDelta, "m1", true, false, "x"), nestedSource())

	assert.Empty(t, rows)
	assert.Nil(t, fx.Live)
}
