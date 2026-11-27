package terminal_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

func TestRegression_SiblingChatsSharingAWorktreeDoNotSeeEachOthersSessions(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()

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

	assert.ElementsMatch(t, []string{sidA}, eng.ListSessionsForChat("chat-a"),
		"chat-a must see only the session it opened")
	assert.ElementsMatch(t, []string{sidB}, eng.ListSessionsForChat("chat-b"),
		"chat-b must see only the session it opened")

	assert.NotContains(t, eng.ListSessionsForChat("chat-a"), sidB,
		"chat-a must NOT see its sibling's session despite sharing a worktree")
	assert.NotContains(t, eng.ListSessionsForChat("chat-b"), sidA,
		"chat-b must NOT see its sibling's session despite sharing a worktree")

	assert.ElementsMatch(t, []string{sidA, sidB}, eng.ListSessions())
}

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

	require.NoError(t, eng.Kill(ctx, sidA))
	require.Eventually(t, func() bool { return !eng.SessionExists(ctx, sidA) }, 10*time.Second, 5*time.Millisecond)

	assert.Empty(t, eng.ListSessionsForChat("chat-a"))
	assert.ElementsMatch(t, []string{sidB}, eng.ListSessionsForChat("chat-b"),
		"the sibling's shell must survive a kill in the chat next door")
	assert.True(t, eng.SessionLive(ctx, sidB), "sibling's PTY must still be running")
}

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

	assert.ElementsMatch(t, []string{sidA}, eng.ListSessionsForChat("chat-a"))
	assert.ElementsMatch(t, []string{sidB}, eng.ListSessionsForChat("chat-b"))

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
