//go:build integration

package v0

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTerminalsDef_SnapshotFromEngine proves the lifecycle topic's snapshot is
// derived from the in-memory engine registry (D6: no terminal_sessions view.db):
// a session created in the engine surfaces as a DTO carrying the OWNING CHAT.
// The subscribing client's scope is the bare chat id on the flat
// /v0/chats/:chatId/terminals route, so that id is what the snapshot is asked
// for and what comes back on the frame.
func TestTerminalsDef_SnapshotFromEngine(t *testing.T) {
	appContainer, engContainer := newAppAndEngine(t)
	ctx := context.Background()

	worktree := t.TempDir()

	snap := terminalsSnapshot(appContainer, engContainer)
	require.NotNil(t, snap)
	assert.Empty(t, snap("c1"), "no sessions yet")

	sid, err := engContainer.Terminal.Create(ctx, "c1", worktree, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = engContainer.Terminal.Kill(ctx, sid) })

	got := snap("c1")
	require.Len(t, got, 1)
	assert.Equal(t, sid, got[0].ID)
	assert.Equal(t, "c1", got[0].ChatID)
	// The session was created but no WS client has attached, so its real engine
	// state is "detached" (Session.State(): "active" requires len(clients) > 0;
	// a live PTY with no clients is "detached"). The snapshot must report that
	// actual state, not assume a freshly-created session is "active".
	assert.Equal(t, "detached", got[0].Status)
}
