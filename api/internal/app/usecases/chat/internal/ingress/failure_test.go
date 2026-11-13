package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestMap_AFailureKeepsItsReasonAndItsReply(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")

	rows, fx := c.mapped(engineagents.CanonicalEvent{
		Kind: engineagents.HookTurnFailed, Message: "I could not",
		Failure: &engineagents.TurnFailure{Reason: "usage_limit", Detail: "out of credit", Kind: "limit"},
	}, hooksSource())

	require.Equal(t, []ledger.Kind{ledger.KindAssistantText, ledger.KindTurnFailed}, kinds(rows))
	assert.Equal(t, "I could not", bodyOf[ledger.AssistantText](t, rows[0]).Text)
	failed := bodyOf[ledger.TurnFailed](t, rows[1])
	assert.Equal(t, "usage_limit", failed.Reason)
	assert.Equal(t, "out of credit", failed.Detail)
	assert.Equal(t, "limit", failed.Kind)
	assert.NotEmpty(t, fx.Ended)

	c2 := liveChatRows(t)
	c2.typed("go")
	rows, _ = c2.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnFailed, Message: "boom"}, hooksSource())
	require.Equal(t, []ledger.Kind{ledger.KindTurnFailed}, kinds(rows), "with no detail, the message is the detail")
	assert.Equal(t, ledger.TurnFailed{Reason: ledger.ReasonTurnFailed, Detail: "boom"}, bodyOf[ledger.TurnFailed](t, rows[0]))
}

func TestMap_AFailureSettlesTheSteerItsTurnCarried(t *testing.T) {
	c := liveChatRows(t)
	c.typed("go")
	c.commit(crowbarPrompt(t, "req-1", "and also", ledger.DeliverySteer))

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnFailed}, hooksSource())

	require.Equal(t, []ledger.Kind{ledger.KindTurnFailed, ledger.KindPromptSettled}, kinds(rows))
	assert.Equal(t, ledger.ReasonTurnFailed, bodyOf[ledger.PromptSettled](t, rows[1]).Reason)
}

// A failure of a turn the chat never saw start took none of the prompts its
// runner carried; a late failure of a closed turn leaves the next prompt alone.
func TestMap_AFailureWithNothingRunningSettlesTheCarriedPrompts(t *testing.T) {
	c := liveChatRows(t)
	c.commit(crowbarPrompt(t, "req-2", "hello", ledger.DeliveryRestart))
	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnFailed, TurnID: "p-1", Message: "401"}, hooksSource())
	require.Equal(t, []ledger.Kind{ledger.KindTurnFailed, ledger.KindPromptSettled}, kinds(rows))
	assert.Equal(t, ledger.ReasonTurnFailed, bodyOf[ledger.PromptSettled](t, rows[1]).Reason)

	late := liveChatRows(t)
	late.typed("go")
	late.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop}, hooksSource())
	late.commit(crowbarPrompt(t, "req-3", "next", ledger.DeliveryRestart))
	rows, _ = late.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnFailed, Message: "late"}, hooksSource())
	assert.Equal(t, []ledger.Kind{ledger.KindTurnFailed}, kinds(rows))
}

func TestNoticeRows_FailTheRunningTurnOrTheCarriedPrompts(t *testing.T) {
	notice := engineagents.TerminalNotice{Kind: "usage_limit", Text: "out of credit"}

	c := liveChatRows(t)
	turnID := c.typed("go")
	c.commit(crowbarPrompt(t, "req-1", "and also", ledger.DeliverySteer))
	rows, err := noticeRows(c.st, "run-1", notice, Unrecorded{Answers: []liveText{{TurnID: turnID, ID: "a1", Text: "partial"}}})
	require.NoError(t, err)
	assert.Equal(t, []ledger.Kind{ledger.KindAssistantText, ledger.KindTurnFailed, ledger.KindPromptSettled}, kinds(rows))

	rows, err = noticeRows(c.st, "run-2", notice, Unrecorded{})
	require.NoError(t, err)
	assert.Empty(t, rows, "another runner's notice ends nothing of this turn")

	idle := liveChatRows(t)
	idle.commit(crowbarPrompt(t, "req-2", "hello", ledger.DeliveryRestart))
	rows, err = noticeRows(idle.st, "run-1", notice, Unrecorded{})
	require.NoError(t, err)
	require.Equal(t, []ledger.Kind{ledger.KindTurnFailed, ledger.KindPromptSettled}, kinds(rows))
	assert.Equal(t, "out of credit", bodyOf[ledger.TurnFailed](t, rows[0]).Detail)
	assert.Equal(t, "limit", bodyOf[ledger.TurnFailed](t, rows[0]).Kind, "a usage-limit notice is the limit kind")
	assert.Equal(t, "usage_limit", bodyOf[ledger.PromptSettled](t, rows[1]).Reason)
}
