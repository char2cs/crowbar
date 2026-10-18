package turn

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func announce(t *testing.T, turns *Turns, provider, sessionID string) {
	t.Helper()
	_, _, err := turns.ledger.Do(t.Context(), "chat-1", func(st ledger.State) ([]ledger.Row, error) {
		return ledger.Announce(st, provider, sessionID, false)
	})
	require.NoError(t, err)
}

func foreign(t *testing.T, turns *Turns, runner engineagents.Runner, kind, sessionID string) bool {
	t.Helper()
	_, bound, err := turns.withHookSession(t.Context(), runner)
	require.NoError(t, err)
	return namesAnotherConversation(bound, engineagents.CanonicalEvent{Kind: kind, SessionID: sessionID})
}

// A child thread or internal session sharing the wire is foreign to the chat's
// bound session; the announcement itself and an id-less event never are.
func TestNamesAnotherConversation_JudgesAgainstTheChatsBoundSession(t *testing.T) {
	t.Parallel()
	turns := &Turns{ledger: newTurnLedger(t)}
	runner := engineagents.Runner{ID: "runner-1", CurrentChatID: "chat-1", ProviderID: "p1"}

	assert.False(t, foreign(t, turns, runner, engineagents.HookTurnStop, "thread-child"),
		"an unbound chat has nothing to judge against, so nothing is dropped")

	announce(t, turns, "p1", "thread-main")
	assert.False(t, foreign(t, turns, runner, engineagents.HookTurnStop, "thread-main"))
	assert.True(t, foreign(t, turns, runner, engineagents.HookTurnStop, "thread-child"))
	assert.False(t, foreign(t, turns, runner, engineagents.HookSessionStart, "thread-child"),
		"session_start is the announcement, never filtered")
	assert.False(t, foreign(t, turns, runner, engineagents.HookTurnStop, ""))

	other := runner
	other.ProviderID = "p2"
	assert.False(t, foreign(t, turns, other, engineagents.HookTurnStop, "thread-child"),
		"another provider's conversation on the chat is its own")

	announce(t, turns, "p1", "thread-cleared")
	assert.True(t, foreign(t, turns, runner, engineagents.HookTurnStop, "thread-main"),
		"a /clear rebinds, and the session left behind is foreign again")
	assert.False(t, foreign(t, turns, runner, engineagents.HookTurnStop, "thread-cleared"))
}

// A ledger that cannot be read fails the hook rather than letting every
// conversation through the filter.
func TestWithHookSession_ALedgerReadErrorIsReturned(t *testing.T) {
	t.Parallel()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	l, err := ledgerstore.Open(db, time.Now)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	runner := engineagents.Runner{ID: "runner-1", CurrentChatID: "chat-1", ProviderID: "p1"}

	_, _, err = (&Turns{ledger: l}).withHookSession(t.Context(), runner)
	require.Error(t, err)
}

// Everything one hook records names the session the hook read, even after the
// ledger has moved on.
func TestBoundSession_AnswersFromTheHooksOwnRead(t *testing.T) {
	t.Parallel()
	turns := &Turns{ledger: newTurnLedger(t)}
	runner := engineagents.Runner{ID: "runner-1", CurrentChatID: "chat-1", ProviderID: "p1"}
	announce(t, turns, "p1", "S1")
	ctx, bound, err := turns.withHookSession(t.Context(), runner)
	require.NoError(t, err)
	announce(t, turns, "p1", "S2")

	assert.Equal(t, "S1", bound)
	assert.Equal(t, "S1", turns.boundSession(ctx, "chat-1", "p1"))
	assert.Equal(t, "S2", turns.boundSession(t.Context(), "chat-1", "p1"), "outside a hook it is read now")
	assert.Empty(t, turns.boundSession(ctx, "chat-1", "p2"), "another provider's is its own read")
}
