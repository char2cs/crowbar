package turn

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func announce(t *testing.T, turns *Turns, chatID, provider, sessionID string) {
	t.Helper()
	_, _, err := turns.ledger.Do(t.Context(), chatID, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.Announce(st, provider, sessionID, func() (bool, error) { return false, nil })
	})
	require.NoError(t, err)
}

func foreign(t *testing.T, turns *Turns, runner engineagents.Runner, kind, sessionID string) bool {
	t.Helper()
	return turns.namesAnotherConversation(t.Context(), runner, engineagents.CanonicalEvent{Kind: kind, SessionID: sessionID})
}

// A child thread or internal session sharing the wire is foreign to the chat's
// bound session; the announcement itself and an id-less event never are.
func TestNamesAnotherConversation_JudgesAgainstTheChatsBoundSession(t *testing.T) {
	t.Parallel()
	turns := &Turns{ledger: newTurnLedger(t)}
	runner := engineagents.Runner{ID: "runner-1", CurrentChatID: "chat-1", ProviderID: "p1"}

	assert.False(t, foreign(t, turns, runner, engineagents.HookTurnStop, "thread-child"),
		"an unbound chat has nothing to judge against, so nothing is dropped")

	announce(t, turns, "chat-1", "p1", "thread-main")
	assert.False(t, foreign(t, turns, runner, engineagents.HookTurnStop, "thread-main"))
	assert.True(t, foreign(t, turns, runner, engineagents.HookTurnStop, "thread-child"))
	assert.False(t, foreign(t, turns, runner, engineagents.HookSessionStart, "thread-child"),
		"session_start is the announcement, never filtered")
	assert.False(t, foreign(t, turns, runner, engineagents.HookTurnStop, ""))

	other := runner
	other.ProviderID = "p2"
	assert.False(t, foreign(t, turns, other, engineagents.HookTurnStop, "thread-child"),
		"another provider's conversation on the chat is its own")

	announce(t, turns, "chat-1", "p1", "thread-cleared")
	assert.True(t, foreign(t, turns, runner, engineagents.HookTurnStop, "thread-main"),
		"a /clear rebinds, and the session left behind is foreign again")
	assert.False(t, foreign(t, turns, runner, engineagents.HookTurnStop, "thread-cleared"))
}

// Guard tests build a bare &Turns{}: with no ledger nothing is foreign.
func TestNamesAnotherConversation_WithoutALedgerDropsNothing(t *testing.T) {
	t.Parallel()
	runner := engineagents.Runner{ID: "runner-1", CurrentChatID: "chat-1", ProviderID: "p1"}
	assert.False(t, foreign(t, &Turns{}, runner, engineagents.HookTurnStop, "thread-child"))
}
