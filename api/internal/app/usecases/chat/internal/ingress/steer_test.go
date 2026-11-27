package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func steering(t *testing.T) *chatRows {
	t.Helper()
	c := liveChatRows(t)
	c.typed("go")
	c.commit(crowbarPrompt(t, "req-1", "and also", ledger.DeliverySteer))
	return c
}

func steerable(s Source) Source {
	s.Steer = func(p ledger.Prompt) ([]byte, bool) { return []byte(`{"steer":"` + p.Text + `"}`), true }
	return s
}

func TestSteer_APendingSteerRidesTheTurnStopReply(t *testing.T) {
	c := steering(t)

	rows, fx := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop}, steerable(hooksSource()))

	require.Equal(t, []ledger.Kind{ledger.KindTurnEnded, ledger.KindTurnStarted}, kinds(rows))
	assert.Equal(t, "T-req-1", rows[1].TurnID, "its turn starts in the commit that ends the last")
	assert.Equal(t, "req-1", bodyOf[ledger.TurnStarted](t, rows[1]).RequestID)
	assert.JSONEq(t, `{"steer":"and also"}`, string(fx.Reply))
	assert.Empty(t, c.st.PendingPrompts())
}

func TestSteer_ASteerTheCLICannotTakeIsSettled(t *testing.T) {
	unheld := steerable(hooksSource())
	unheld.Held = false
	refused := hooksSource()
	refused.Steer = func(ledger.Prompt) ([]byte, bool) { return nil, false }

	for name, src := range map[string]Source{"no reply": hooksSource(), "refused": refused, "unheld": unheld} {
		t.Run(name, func(t *testing.T) {
			c := steering(t)

			rows, fx := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop}, src)

			require.Equal(t, []ledger.Kind{ledger.KindTurnEnded, ledger.KindPromptSettled}, kinds(rows))
			assert.Equal(t, ledger.ReasonDeliveryFailed, bodyOf[ledger.PromptSettled](t, rows[1]).Reason)
			assert.Empty(t, fx.Reply)
		})
	}
}

func TestSteer_AnEndThatClosesNothingHandsOverNothing(t *testing.T) {
	c := steering(t)
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, TurnID: "pt-1"}, hooksSource())

	rows, fx := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, TurnID: "pt-1"}, steerable(hooksSource()))

	assert.Empty(t, rows, "a late end")
	assert.Empty(t, fx.Reply)
}

func TestSteer_QueuedSteersRideSuccessiveTurnEnds(t *testing.T) {
	c := steering(t)
	c.commit(crowbarPrompt(t, "req-2", "then this", ledger.DeliverySteer))

	rows, fx := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop}, steerable(hooksSource()))

	require.Equal(t, []ledger.Kind{ledger.KindTurnEnded, ledger.KindTurnStarted}, kinds(rows))
	assert.Equal(t, "req-1", bodyOf[ledger.TurnStarted](t, rows[1]).RequestID)
	assert.JSONEq(t, `{"steer":"and also"}`, string(fx.Reply))
	require.Len(t, c.st.PendingPrompts(), 1)
	assert.Equal(t, "req-2", c.st.PendingPrompts()[0].RequestID)
}

func TestRegression_Steer_StopSettlesEveryQueuedSteerAndTheLateEndDeliversNone(t *testing.T) {
	c := steering(t)
	c.commit(crowbarPrompt(t, "req-2", "then this", ledger.DeliverySteer))
	stopped, err := ledger.Stop(c.st, "")
	require.NoError(t, err)
	c.commit(stopped...)

	settled := map[string]string{}
	for _, r := range stopped {
		if r.Kind == ledger.KindPromptSettled {
			body := bodyOf[ledger.PromptSettled](t, r)
			settled[body.RequestID] = body.Reason
		}
	}
	assert.Equal(t, map[string]string{"req-1": ledger.ReasonStopped, "req-2": ledger.ReasonStopped}, settled)
	assert.Empty(t, c.st.PendingPrompts())

	rows, fx := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, TurnID: "pt-1"}, steerable(hooksSource()))

	assert.NotContains(t, kinds(rows), ledger.KindTurnStarted)
	assert.Empty(t, fx.Reply)
	_, running := c.st.RunningTurn()
	assert.False(t, running)
}

func TestRegression_Steer_AProviderPromptWithTheSameWordsDoesNotBindToAQueuedSteer(t *testing.T) {
	c := steering(t)
	c.commit(crowbarPrompt(t, "req-2", "and also", ledger.DeliverySteer))
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop}, steerable(hooksSource()))
	require.Len(t, c.st.PendingPrompts(), 1)
	echo := hooksSource()
	echo.Prompt = Prompt{Text: "and also", Origin: ledger.OriginProvider}

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, TurnID: "pt-2"}, echo)

	for _, r := range rows {
		if r.Kind == ledger.KindTurnStarted {
			assert.Empty(t, bodyOf[ledger.TurnStarted](t, r).RequestID, "the echo opens a turn of its own")
			assert.NotEqual(t, "T-req-2", r.TurnID)
		}
	}
	require.Len(t, c.st.PendingPrompts(), 1)
	assert.Equal(t, "req-2", c.st.PendingPrompts()[0].RequestID, "the steer is still waiting for its own turn end")
}

func TestRegression_Steer_ALateMessageOfAnEndedTurnJoinsItsReplyNotTheNextTurn(t *testing.T) {
	c := steering(t)
	c.commit(crowbarPrompt(t, "req-2", "then this", ledger.DeliverySteer))
	stop := func(reply string) []ledger.Row {
		rows, _ := c.mapped(engineagents.CanonicalEvent{
			Kind: engineagents.HookTurnStop, TurnID: "pt-1", Message: reply,
		}, steerable(hooksSource()))
		return rows
	}
	first, _ := c.st.RunningTurn()
	stop("R1")
	second, _ := c.st.RunningTurn()
	late := func(id, words string) ledger.Row {
		rows, _ := c.mapped(delta(engineagents.HookMessageDelta, id, true, true, words), hooksSource())
		require.Len(t, rows, 1)
		return rows[0]
	}

	r1 := late("m-1", "R1")

	assert.Equal(t, first.ID, r1.TurnID, "the turn that said it, though another is running")
	assert.Equal(t, ledger.ReplyID(first.ID), bodyOf[ledger.AssistantText](t, r1).Reply)

	stop("SA")
	third, _ := c.st.RunningTurn()
	sa := late("m-2", "SA")

	assert.Equal(t, second.ID, sa.TurnID)
	assert.Equal(t, ledger.ReplyID(second.ID), bodyOf[ledger.AssistantText](t, sa).Reply)

	sb := late("m-3", "SA")

	assert.Equal(t, third.ID, sb.TurnID, "a reply already said is claimed once: the running turn's own words stay its own")
	assert.Empty(t, bodyOf[ledger.AssistantText](t, sb).Reply)
}
