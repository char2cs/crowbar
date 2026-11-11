package terminal_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

func TestEngine_Create_And_ListSessions(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	assert.Empty(t, eng.ListSessions())

	sid, err := eng.Create(ctx, "chat-1", dir, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, sid)

	assert.Contains(t, eng.ListSessions(), sid)

	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_Create_KeysSessionByChat(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid1, err := eng.Create(ctx, "chat-a", dir, nil)
	require.NoError(t, err)
	sid2, err := eng.Create(ctx, "chat-b", dir, nil)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{sid1}, eng.ListSessionsForChat("chat-a"))
	assert.ElementsMatch(t, []string{sid2}, eng.ListSessionsForChat("chat-b"))

	require.NoError(t, eng.Kill(ctx, sid1))
	require.NoError(t, eng.Kill(ctx, sid2))
}

func TestEngine_ListSessionsForChat(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	assert.Empty(t, eng.ListSessionsForChat("chat-a"))

	sid, err := eng.Create(ctx, "chat-a", dir, nil)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{sid}, eng.ListSessionsForChat("chat-a"))
	assert.Empty(t, eng.ListSessionsForChat("chat-other"))

	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_Write_And_Resize_Unknown(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	require.Error(t, eng.Write(ctx, "nope", []byte("hi")))
	assert.Error(t, eng.Resize(ctx, "nope", 80, 24))
}

func TestEngine_Write(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-1", dir, nil)
	require.NoError(t, err)

	require.NoError(t, eng.Write(ctx, sid, []byte("echo ok\n")))
	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_Resize(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-1", dir, nil)
	require.NoError(t, err)

	require.NoError(t, eng.Resize(ctx, sid, 120, 40))
	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_ListSessions_AfterKill(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-1", dir, nil)
	require.NoError(t, err)
	require.NoError(t, eng.Kill(ctx, sid))

	assert.NotContains(t, eng.ListSessions(), sid)
}

func TestEngine_SessionExists(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	assert.False(t, eng.SessionExists(ctx, "nonexistent"))

	sid, err := eng.Create(ctx, "chat-1", dir, nil)
	require.NoError(t, err)

	assert.True(t, eng.SessionExists(ctx, sid))
	require.NoError(t, eng.Kill(ctx, sid))
	assert.False(t, eng.SessionExists(ctx, sid))
}

// TestEngine_LoadPlaceholder_IsPlaceholder verifies that after LoadPlaceholder the
// session appears in the registry as a suspended (non-live) placeholder.
func TestEngine_LoadPlaceholder_IsPlaceholder(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()

	sid := "ph-sess-1"
	err := eng.LoadPlaceholder(ctx, terminal.SessionMeta{
		SessionID: sid,
		ChatID:    "chat-ph",
		CWD:       "/tmp",
		Shell:     "/bin/sh",
		ProfileID: "",
		State:     "suspended",
	}, []byte("old scrollback"))
	require.NoError(t, err)

	assert.True(t, eng.SessionExists(ctx, sid), "placeholder must be present in registry")
	state, ok := eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "suspended", state, "placeholder state must be 'suspended'")

	// Cleanup.
	require.NoError(t, eng.Kill(ctx, sid))
}

// TestEngine_LoadPlaceholder_Idempotent verifies that loading the same session ID
// twice does not double-register and does not error.
func TestEngine_LoadPlaceholder_Idempotent(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()

	sid := "ph-sess-idem"
	meta := terminal.SessionMeta{
		SessionID: sid,
		ChatID:    "chat-ph",
		CWD:       "/tmp",
		Shell:     "/bin/sh",
	}

	require.NoError(t, eng.LoadPlaceholder(ctx, meta, nil))
	require.NoError(t, eng.LoadPlaceholder(ctx, meta, nil)) // second call must be no-op

	// Should still appear exactly once.
	ids := eng.ListSessions()
	count := 0
	for _, id := range ids {
		if id == sid {
			count++
		}
	}
	assert.Equal(t, 1, count, "session must be registered exactly once after two loads")

	require.NoError(t, eng.Kill(ctx, sid))
}

// TestEngine_StateOf_ReturnsStateForLiveSession verifies StateOf returns
// "detached" for a freshly created session (no clients yet).
func TestEngine_StateOf_ReturnsStateForLiveSession(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-stateof", dir, nil)
	require.NoError(t, err)

	state, ok := eng.StateOf(sid)
	assert.True(t, ok, "StateOf must return ok=true for a live session")
	// Newly created session has no clients → "detached"
	assert.Equal(t, "detached", state)

	require.NoError(t, eng.Kill(ctx, sid))
}

// TestEngine_StateOf_ReturnsFalseForUnknown verifies StateOf returns ("", false)
// for a session that does not exist in the registry.
func TestEngine_StateOf_ReturnsFalseForUnknown(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)

	state, ok := eng.StateOf("no-such-session")
	assert.False(t, ok)
	assert.Empty(t, state)
}

// TestEngine_StateOf_ActiveWhileAttached verifies StateOf returns "active"
// while a client is attached.
func TestEngine_StateOf_ActiveWhileAttached(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-active", dir, nil)
	require.NoError(t, err)

	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()

	// Wait until we receive at least one frame, confirming the client is registered.
	// blocks on the fan-out signal — must receive initial output to confirm attach
	waitForMsg(t, conn, func(d string) bool { return len(d) > 0 })

	state, ok := eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "active", state)

	_ = conn.Close()
	<-attachDone
	require.NoError(t, eng.Kill(ctx, sid))
}

// TestRegression_TerminalLifecycle_StateOf pins the lifecycle wire-protocol data path:
// StateOf must reflect the session's actual state through attach, last-client
// detach, Suspend and Kill.
func TestRegression_TerminalLifecycle_StateOf(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	// Create → should be detached (no clients). Block until the shell is at its prompt, so the
	// Suspend later in this test meets the idle gate its eligibility check applies.
	sid := newReadyShell(t, eng, "chat-regress", dir)

	state, ok := eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "detached", state, "newly created session must report detached (no clients)")

	// Attach client → "active".
	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()
	// blocks on the fan-out signal — must receive initial output to confirm attach
	waitForMsg(t, conn, func(d string) bool { return len(d) > 0 })

	state, ok = eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "active", state, "session must be active while client is attached")

	// Detach client → "detached", persisted as such.
	_ = conn.Close()
	<-attachDone
	require.True(t, store.hasSavedWithState(sid, "detached"),
		"the last client leaving persists a detached row")

	state, ok = eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "detached", state)

	// Suspend → "suspended". The shell is at its prompt (idle) and now
	// detached, so Suspend's eligibility gate is satisfied and ONE call must take.
	require.NoError(t, eng.Suspend(ctx, sid))
	require.True(t, store.hasSavedWithState(sid, "suspended"),
		"Suspend persists the suspended meta before returning")

	state, ok = eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "suspended", state)

	// Kill the placeholder → gone.
	require.NoError(t, eng.Kill(ctx, sid))
	_, ok = eng.StateOf(sid)
	assert.False(t, ok, "a killed placeholder leaves the registry")
}
