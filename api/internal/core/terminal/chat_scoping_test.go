package terminal_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

// TestRegression_SiblingChatsSharingAWorktreeDoNotSeeEachOthersSessions is the
// behavioural guarantee the workspace→chat re-key exists to deliver, and it is
// deliberately written as the bug it fixes rather than as a rename check.
//
// THE BUG: the engine used to key sessions by WORKSPACE. Two chats that share
// one worktree — which chat-first creation makes the COMMON case, since batch
// import and repo-add both mint many chats over shared lineage — therefore
// shared one session bucket. `GET .../terminals` for either chat listed BOTH
// chats' shells, so one chat could see, attach to, and DELETE a shell the other
// one opened.
//
// Note what this test does NOT do: it does not give the two chats separate
// directories. They are handed the SAME `shared` worktree path, because a test
// that separated them would prove nothing — it would pass just as happily
// against the old workspace-keyed engine. Sharing the directory while NOT
// sharing the sessions is the whole claim.
func TestRegression_SiblingChatsSharingAWorktreeDoNotSeeEachOthersSessions(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()

	// ONE worktree. Both chats resolve to it — that is what being siblings on a
	// shared worktree means, and it is exactly the situation that used to leak.
	shared := t.TempDir()

	sidA, err := eng.Create(ctx, "chat-a", shared, nil)
	require.NoError(t, err)
	sidB, err := eng.Create(ctx, "chat-b", shared, nil)
	require.NoError(t, err)
	require.NotEqual(t, sidA, sidB)

	t.Cleanup(func() {
		_ = eng.Kill(ctx, sidA)
		_ = eng.Kill(ctx, sidB)
	})

	// Each chat sees exactly its own session, and nothing of its sibling's.
	assert.ElementsMatch(t, []string{sidA}, eng.ListSessionsForChat("chat-a"),
		"chat-a must see only the session it opened")
	assert.ElementsMatch(t, []string{sidB}, eng.ListSessionsForChat("chat-b"),
		"chat-b must see only the session it opened")

	assert.NotContains(t, eng.ListSessionsForChat("chat-a"), sidB,
		"chat-a must NOT see its sibling's session despite sharing a worktree")
	assert.NotContains(t, eng.ListSessionsForChat("chat-b"), sidA,
		"chat-b must NOT see its sibling's session despite sharing a worktree")

	// Both really are registered — proving the disjointness above is genuine
	// scoping and not two lookups that both silently missed.
	assert.ElementsMatch(t, []string{sidA, sidB}, eng.ListSessions())
}

// TestRegression_KillingOneChatsSessionLeavesItsSiblingsAlive proves the
// isolation is not merely a read-side filter: a chat tearing its own shell down
// must not disturb the sibling sharing its worktree.
//
// Under the old workspace key the two sessions lived in one bucket, so any
// verb that resolved "this workspace's sessions" — the cascade reaper included
// — operated on both.
func TestRegression_KillingOneChatsSessionLeavesItsSiblingsAlive(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	shared := t.TempDir()

	sidA, err := eng.Create(ctx, "chat-a", shared, nil)
	require.NoError(t, err)
	sidB, err := eng.Create(ctx, "chat-b", shared, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Kill(ctx, sidB) })

	// Block on the engine's own ended signal for A, never on a clock: the reap
	// is what makes the assertion below meaningful, so the test waits for the
	// engine to say it happened.
	endedCh := make(chan string, 2)
	eng.OnSessionEnded(func(_ context.Context, chatID, sid string, _ int) {
		endedCh <- chatID + "/" + sid
	})

	require.NoError(t, eng.Kill(ctx, sidA))
	assert.Equal(t, "chat-a/"+sidA, <-endedCh,
		"the ended frame must name the OWNING chat, not the shared workspace")

	assert.Empty(t, eng.ListSessionsForChat("chat-a"))
	assert.ElementsMatch(t, []string{sidB}, eng.ListSessionsForChat("chat-b"),
		"the sibling's shell must survive a kill in the chat next door")
	assert.True(t, eng.SessionLive(ctx, sidB), "sibling's PTY must still be running")
}

// TestRegression_ChatScopedSessionsPersistUnderTheirOwningChat proves the
// durable metadata follows the same owner, so a restart cannot re-pool two
// siblings' scrollback under one key.
//
// It drives persistence through Shutdown rather than Suspend on purpose.
// Suspend is IDLE-GATED — it returns nil without doing anything for a session
// that is not idle yet — so a freshly spawned shell still painting its prompt
// would make this test's real subject (what ChatID lands in the row) depend on
// process timing. Shutdown's graceful-persist path has no such gate: it flushes
// and writes meta for every live persistable session, unconditionally, and
// blocks until every reap is home. Same assertion, no clock.
func TestRegression_ChatScopedSessionsPersistUnderTheirOwningChat(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	ctx := context.Background()
	shared := t.TempDir()

	sidA, err := eng.Create(ctx, "chat-a", shared, nil)
	require.NoError(t, err)
	sidB, err := eng.Create(ctx, "chat-b", shared, nil)
	require.NoError(t, err)

	// Scoping holds while both are live.
	assert.ElementsMatch(t, []string{sidA}, eng.ListSessionsForChat("chat-a"))
	assert.ElementsMatch(t, []string{sidB}, eng.ListSessionsForChat("chat-b"))

	// Shutdown BLOCKS until every session is persisted and reaped, so the rows
	// below are complete by the time it returns — nothing here waits on a timer.
	eng.Shutdown()

	owners := make(map[string]string)
	for _, row := range store.liveRows() {
		owners[row.SessionID] = row.ChatID
	}
	require.Len(t, owners, 2, "both sessions must have been persisted by Shutdown")
	assert.Equal(t, "chat-a", owners[sidA], "persisted row must record the OWNING CHAT, not the shared workspace")
	assert.Equal(t, "chat-b", owners[sidB], "persisted row must record the OWNING CHAT, not the shared workspace")
	assert.NotEqual(t, owners[sidA], owners[sidB],
		"two siblings on one worktree must not persist under a single shared key")
}
