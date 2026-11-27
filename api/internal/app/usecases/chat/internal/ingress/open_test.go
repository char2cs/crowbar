package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestRegression_OpenProviderTurn_ATypedPromptNamingTheEndedTurnOpensItsOwn(t *testing.T) {
	c := liveChatRows(t)
	c.typed("R1")
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, TurnID: "pt-1"}, hooksSource())
	c.commit(crowbarPrompt(t, "req-2", "R2", ledger.DeliveryType))
	stale := hooksSource()
	stale.Prompt = Prompt{Text: "R2", Origin: ledger.OriginProvider}

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, TurnID: "pt-1"}, stale)

	require.Equal(t, []ledger.Kind{ledger.KindTurnStarted}, kinds(rows))
	assert.Equal(t, "req-2", bodyOf[ledger.TurnStarted](t, rows[0]).RequestID)
	assert.Empty(t, c.st.PendingPrompts(), "its turn is the prompt's acknowledgement")
	running, ok := c.st.RunningTurn()
	require.True(t, ok)

	ended, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, TurnID: "pt-2"}, hooksSource())

	assert.Equal(t, running.ID, ended[0].TurnID, "the turn's own end, under the id its hooks carry from here")
	_, stillRunning := c.st.RunningTurn()
	assert.False(t, stillRunning)
}

func TestOpenProviderTurn_ARetryAfterTheTurnEndedOpensNothing(t *testing.T) {
	c := liveChatRows(t)
	c.typed("R1")
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, TurnID: "pt-1"}, hooksSource())
	again := hooksSource()
	again.Prompt = Prompt{Text: "R1", Origin: ledger.OriginProvider}

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, TurnID: "pt-1"}, again)

	assert.Empty(t, rows)
}

func TestRegression_OpenProviderTurn_ATypedPromptNamingAnEndedTurnsOwnEndId(t *testing.T) {
	c := liveChatRows(t)
	c.typed("R1")
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, TurnID: "pt-1"}, hooksSource())
	c.commit(crowbarPrompt(t, "req-2", "R2", ledger.DeliveryType))
	stale := hooksSource()
	stale.Prompt = Prompt{Text: "R2", Origin: ledger.OriginProvider}
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, TurnID: "pt-1"}, stale)
	c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, TurnID: "pt-2"}, hooksSource())
	c.commit(crowbarPrompt(t, "req-3", "R3", ledger.DeliveryType))
	stale3 := hooksSource()
	stale3.Prompt = Prompt{Text: "R3", Origin: ledger.OriginProvider}

	rows, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, TurnID: "pt-2"}, stale3)

	require.Equal(t, []ledger.Kind{ledger.KindTurnStarted}, kinds(rows))
	assert.Empty(t, bodyOf[ledger.TurnStarted](t, rows[0]).ProviderTurnID, "pt-2 is the ended turn's, not this one's")
	running, ok := c.st.RunningTurn()
	require.True(t, ok)

	ended, _ := c.mapped(engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, TurnID: "pt-3"}, hooksSource())

	require.NotEmpty(t, ended)
	assert.Equal(t, running.ID, ended[len(ended)-1].TurnID, "its own end, not a turn the ledger never ran")
}
