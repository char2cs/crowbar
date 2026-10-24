package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func announced(t *testing.T, st ledger.State, provider, sessionID string) ledger.State {
	t.Helper()
	rows, err := ledger.Announce(st, provider, sessionID, false)
	require.NoError(t, err)
	for _, r := range rows {
		r.Seq = st.Head() + 1
		st.Apply(r)
	}
	return st
}

// A nested conversation or internal session sharing the wire is foreign to the
// chat's bound session; the announcement itself and an id-less event never are.
func TestNamesAnotherConversation_JudgesAgainstTheChatsBoundSession(t *testing.T) {
	t.Parallel()
	foreign := func(st ledger.State, provider, kind, sessionID string) bool {
		return namesAnotherConversation(boundSession(st, provider), engineagents.CanonicalEvent{Kind: kind, SessionID: sessionID})
	}
	st := ledger.Fold("chat-1", nil)
	assert.False(t, foreign(st, "p1", engineagents.HookTurnStop, "thread-child"),
		"an unbound chat has nothing to judge against, so nothing is dropped")

	st = announced(t, st, "p1", "thread-main")
	assert.False(t, foreign(st, "p1", engineagents.HookTurnStop, "thread-main"))
	assert.True(t, foreign(st, "p1", engineagents.HookTurnStop, "thread-child"))
	assert.False(t, foreign(st, "p1", engineagents.HookSessionStart, "thread-child"),
		"session_start is the announcement, never judged")
	assert.False(t, foreign(st, "p1", engineagents.HookTurnStop, ""))
	assert.False(t, foreign(st, "p2", engineagents.HookTurnStop, "thread-child"),
		"another provider's conversation on the chat is its own")

	st = announced(t, st, "p1", "thread-cleared")
	assert.True(t, foreign(st, "p1", engineagents.HookTurnStop, "thread-main"),
		"a rebind leaves the session behind foreign")
	assert.False(t, foreign(st, "p1", engineagents.HookTurnStop, "thread-cleared"))
}
