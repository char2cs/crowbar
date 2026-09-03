package runner

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// stubRunnerStoreForResumable answers only ConversationsForChat — the one call
// resumableConversation makes on the runner store. The embedded nil interface
// panics on anything else, so a test relying on a second method fails loudly
// instead of silently zero-valuing.
type stubRunnerStoreForResumable struct {
	agentrunner.EventStore
	convs []engineagents.ChatConversation
}

func (s stubRunnerStoreForResumable) ConversationsForChat(
	context.Context, string,
) ([]engineagents.ChatConversation, error) {
	return s.convs, nil
}

// TestRegression_ResumableConversation_OldConversationWithNoRecordedTurns_ResumesAnyway
// is the fix for the bug that bricked every pre-migration chat in production: PR #151
// introduced agent_turns (queried here via activity.LastTurnForSession) to record turns
// from hooks, but that table has zero rows for any conversation that happened before it
// existed — which looks IDENTICAL to a provider that announced a session and crashed
// before its first turn (the guard's original, legitimate target). Every old chat's real,
// resumable session id was being thrown away and a blank one spawned in its place.
//
// The fix distinguishes the two by age: a conversation first seen long ago (like this
// one, weeks old) is trusted and resumed anyway, falling back to chat.LastActivityAt as
// the gap cutoff since there is no per-turn record to be more precise with.
func TestRegression_ResumableConversation_OldConversationWithNoRecordedTurns_ResumesAnyway(t *testing.T) {
	weeksAgo := time.Now().Add(-21 * 24 * time.Hour)
	lastActivity := time.Now().Add(-2 * time.Hour)

	rs := &Runners{
		runnerStore: stubRunnerStoreForResumable{convs: []engineagents.ChatConversation{
			{ChatID: "chat-1", ProviderID: "claude", SessionID: "sid-legacy-session", FirstSeenAt: weeksAgo},
		}},
		activity: stubActivityForAttach{found: false},
	}
	chat := domain.Chat{ID: "chat-1", LastActivityAt: lastActivity}

	sessionID, leftAt, err := rs.resumableConversation(context.Background(), chat, "claude")

	require.NoError(t, err)
	assert.Equal(t, "sid-legacy-session", sessionID,
		"the real, pre-migration session id must be trusted and resumed, not thrown away")
	assert.True(t, leftAt.Equal(lastActivity),
		"with no per-turn record to draw the gap from, chat.LastActivityAt must stand in for it; got %v want %v",
		leftAt, lastActivity)
}

// TestRegression_ResumableConversation_RecentConversationWithNoRecordedTurns_StillSpawnsFresh
// is the regression-of-the-regression-guard: the fix above must not also swallow the
// genuine race it was carved out of. A provider that announces a session and crashes
// before completing its first turn looks — in the activity table — identical to an old
// conversation: zero rows either way. Only recency tells them apart, and a session
// announced moments ago with nothing recorded is still that crash, not an old
// conversation, and must still be refused so a fresh one is spawned instead.
func TestRegression_ResumableConversation_RecentConversationWithNoRecordedTurns_StillSpawnsFresh(t *testing.T) {
	justNow := time.Now().Add(-5 * time.Second)

	rs := &Runners{
		runnerStore: stubRunnerStoreForResumable{convs: []engineagents.ChatConversation{
			{ChatID: "chat-1", ProviderID: "claude", SessionID: "sid-crashed-session", FirstSeenAt: justNow},
		}},
		activity: stubActivityForAttach{found: false},
	}
	chat := domain.Chat{ID: "chat-1", LastActivityAt: time.Now().Add(-30 * 24 * time.Hour)}

	sessionID, leftAt, err := rs.resumableConversation(context.Background(), chat, "claude")

	require.NoError(t, err)
	assert.Empty(t, sessionID, "a session announced moments ago with no recorded turn is still the crash race, not an old conversation")
	assert.True(t, leftAt.IsZero(), "a refused resume carries no gap cutoff")
}
