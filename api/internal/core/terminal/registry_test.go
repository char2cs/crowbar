package terminal_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

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

func TestEngine_Write_IntoAStdinNobodyReadsEndsWithItsContext(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	sid, err := eng.CreateCommand(t.Context(), "chat-stuck", t.TempDir(),
		[]string{"/bin/sh", "-c", "stty raw -echo; printf STDIN-IGNORED; exec sleep 9999"}, os.Environ(), nil)
	require.NoError(t, err)
	conn := newMockConn()
	go func() { _ = eng.Attach(t.Context(), sid, conn) }()
	waitForMsg(t, conn, func(data string) bool { return strings.Contains(data, "STDIN-IGNORED") })

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, eng.Write(ctx, sid, []byte("x")), context.Canceled, "a cancelled write writes nothing")

	ctx, cancel = context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, eng.Write(ctx, sid, bytes.Repeat([]byte("x"), 4<<20)), context.DeadlineExceeded)
	_ = conn.Close()
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

	require.NoError(t, eng.Kill(ctx, sid))
}

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
	require.NoError(t, eng.LoadPlaceholder(ctx, meta, nil))

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

func TestEngine_StateOf_ReturnsStateForLiveSession(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	sid, err := eng.Create(ctx, "chat-stateof", dir, nil)
	require.NoError(t, err)

	state, ok := eng.StateOf(sid)
	assert.True(t, ok, "StateOf must return ok=true for a live session")
	assert.Equal(t, "detached", state)

	require.NoError(t, eng.Kill(ctx, sid))
}

func TestEngine_StateOf_ReturnsFalseForUnknown(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)

	state, ok := eng.StateOf("no-such-session")
	assert.False(t, ok)
	assert.Empty(t, state)
}

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

	waitForMsg(t, conn, func(d string) bool { return len(d) > 0 })

	state, ok := eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "active", state)

	_ = conn.Close()
	<-attachDone
	require.NoError(t, eng.Kill(ctx, sid))
}

func TestRegression_TerminalLifecycle_StateOf(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	sid := newReadyShell(t, eng, "chat-regress", dir)

	state, ok := eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "detached", state, "newly created session must report detached (no clients)")

	conn := newMockConn()
	attachDone := make(chan struct{})
	go func() {
		defer close(attachDone)
		_ = eng.Attach(ctx, sid, conn)
	}()
	waitForMsg(t, conn, func(d string) bool { return len(d) > 0 })

	state, ok = eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "active", state, "session must be active while client is attached")

	_ = conn.Close()
	<-attachDone
	require.True(t, store.hasSavedWithState(sid, "detached"),
		"the last client leaving persists a detached row")

	state, ok = eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "detached", state)

	require.NoError(t, eng.Suspend(ctx, sid))
	require.True(t, store.hasSavedWithState(sid, "suspended"),
		"Suspend persists the suspended meta before returning")

	state, ok = eng.StateOf(sid)
	assert.True(t, ok)
	assert.Equal(t, "suspended", state)

	require.NoError(t, eng.Kill(ctx, sid))
	_, ok = eng.StateOf(sid)
	assert.False(t, ok, "a killed placeholder leaves the registry")
}
