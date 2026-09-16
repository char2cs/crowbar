package v0

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/engine"
)

// newAppAndEngineForSnapshot mirrors newAppForSnapshot but also returns the
// engine.Container terminalsSnapshot and lspSnapshot need directly (unlike the
// other snapshot sources, which only ever read through appContainer).
func newAppAndEngineForSnapshot(
	t *testing.T,
) (*app.Container, *engine.Container) {
	t.Helper()
	ctx := context.Background()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	a, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)
	return a, eng
}

// TestTerminalsSnapshot_NilEngineReturnsNil covers the guard at the top of
// terminalsSnapshot: a container built without the terminal engine wired
// (nil Container, or Container.Terminal nil) must not construct a source
// function at all, matching lspSnapshot's identical guard for an absent LSP
// engine.
func TestTerminalsSnapshot_NilEngineReturnsNil(t *testing.T) {
	a, _ := newAppAndEngineForSnapshot(t)

	assert.Nil(t, terminalsSnapshot(a, nil))
	assert.Nil(t, terminalsSnapshot(a, &engine.Container{}))
}

// TestTerminalsSnapshot_EmptyScopeReturnsNil covers a subscription that names no
// chat at all. Terminals are chat-scoped, so the scope IS the bare chat id; with
// nothing to key on, the snapshot must yield nil rather than fall back to
// enumerating the whole registry.
func TestTerminalsSnapshot_EmptyScopeReturnsNil(t *testing.T) {
	a, eng := newAppAndEngineForSnapshot(t)
	snap := terminalsSnapshot(a, eng)
	require.NotNil(t, snap)

	assert.Nil(t, snap(""))
}

// TestTerminalsSnapshot_ListsLiveSessionForItsChat is the ordinary case: a
// session created directly in the engine registry (D6: terminals are ephemeral,
// no view.db) appears in the snapshot for the chat that OWNS it, carrying that
// chat id.
//
// The sibling half is the load-bearing one. Both chats are handed the SAME
// worktree, because a fixture that gave them separate directories would pass
// just as happily against a workspace-keyed snapshot: sharing the worktree while
// NOT sharing the replay is the whole claim (see
// core/terminal chat_scoping_test.go for the engine-level twin of this).
func TestTerminalsSnapshot_ListsLiveSessionForItsChat(t *testing.T) {
	a, eng := newAppAndEngineForSnapshot(t)
	ctx := context.Background()

	snap := terminalsSnapshot(a, eng)
	require.NotNil(t, snap)
	assert.Empty(t, snap("chat-a"), "no sessions yet")

	shared := t.TempDir()
	sidA, err := eng.Terminal.Create(ctx, "chat-a", shared, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Terminal.Kill(ctx, sidA) })
	sidB, err := eng.Terminal.Create(ctx, "chat-b", shared, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Terminal.Kill(ctx, sidB) })

	got := snap("chat-a")
	require.Len(t, got, 1)
	assert.Equal(t, sidA, got[0].ID)
	assert.Equal(t, "chat-a", got[0].ChatID)

	// The replayed status is the engine's REAL state, not a constant: a session
	// with no attached client rests "detached", so hardcoding "active" here would
	// pin a guess rather than the passthrough terminalsSnapshot actually does.
	state, ok := eng.Terminal.StateOf(sidA)
	require.True(t, ok)
	assert.Equal(t, state, got[0].Status)

	ids := make([]string, len(got))
	for i, d := range got {
		ids[i] = d.ID
	}
	assert.NotContains(t, ids, sidB,
		"a chat must not replay its sibling's session despite sharing a worktree")
}

// TestTerminalsSnapshot_UnknownChatScopeIsEmpty proves a scope naming a chat
// that owns nothing replays nothing — never the whole registry — even while
// another chat's session is live.
func TestTerminalsSnapshot_UnknownChatScopeIsEmpty(t *testing.T) {
	a, eng := newAppAndEngineForSnapshot(t)
	ctx := context.Background()

	sid, err := eng.Terminal.Create(ctx, "chat-a", t.TempDir(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = eng.Terminal.Kill(ctx, sid) })

	assert.Empty(t, terminalsSnapshot(a, eng)("chat-nobody"))
}
