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

// A steer the ending CLI cannot take now is settled, for its client to keep.
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

	assert.Equal(t, []ledger.Kind{ledger.KindTurnEnded}, kinds(rows), "a late end")
	assert.Empty(t, fx.Reply)
}
