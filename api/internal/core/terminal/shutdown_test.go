package terminal_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestShutdown_FlushPersistNoBufDelete(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	const N = 3
	sids := make([]string, N)
	for i := range sids {
		sids[i] = newReadyShell(t, eng, "chat-graceful-shutdown", store.dir)
	}

	eng.Shutdown()

	for _, sid := range sids {
		assert.True(t, bufExists(store.dir, sid),
			".buf must exist after Shutdown for sid=%s", sid)

		assert.True(t, store.hasSavedWithState(sid, "suspended"),
			"meta must be saved with state=suspended after Shutdown for sid=%s", sid)
	}

	for _, sid := range sids {
		assert.False(t, store.hasDeleted(sid),
			"meta Delete must NOT be called during Shutdown (sid=%s)", sid)
	}

	eng2 := terminal.New()
	terminal.StopMaintenanceForTest(eng2)
	for _, sid := range sids {
		loadErr := eng2.LoadPlaceholder(ctx, terminal.SessionMeta{
			SessionID: sid,
			ChatID:    "chat-graceful-shutdown",
			CWD:       store.dir,
			Shell:     "/bin/sh",
			State:     "suspended",
		}, nil)
		require.NoError(t, loadErr)
		assert.True(t, eng2.SessionExists(ctx, sid),
			"session must be present in fresh engine after LoadPlaceholder (sid=%s)", sid)
		state, ok := eng2.StateOf(sid)
		assert.True(t, ok)
		assert.Equal(t, "suspended", state,
			"session state must be 'suspended' in fresh engine (sid=%s)", sid)
	}

	for _, sid := range sids {
		_ = eng2.Kill(ctx, sid)
	}
	eng2.Shutdown()
}

func TestShutdown_NoDeleteForPlaceholder(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	const sid = "ph-graceful-shutdown"
	scrollback := []byte("old prompt output\r\n")

	require.NoError(t, eng.LoadPlaceholder(ctx, terminal.SessionMeta{
		SessionID: sid,
		ChatID:    "chat-ph-shutdown",
		CWD:       store.dir,
		Shell:     "/bin/sh",
		State:     "suspended",
	}, scrollback))

	bufPath := filepath.Join(store.dir, sid+".buf")
	require.NoError(t, os.WriteFile(bufPath, scrollback, 0o644))

	eng.Shutdown()

	assert.True(t, bufExists(store.dir, sid),
		".buf must NOT be deleted for a placeholder session during Shutdown")

	assert.False(t, store.hasDeleted(sid),
		"Delete must NOT be called for a placeholder session during Shutdown")
}

func TestShutdown_JoinsExitCallbacks(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()

	var exited atomic.Bool
	id, err := eng.CreateCommand(ctx, "chat-exit-drain", t.TempDir(),
		[]string{"cat"}, os.Environ(), func(terminal.Exit) { exited.Store(true) })
	require.NoError(t, err)
	require.True(t, eng.SessionExists(ctx, id))

	eng.Shutdown()

	assert.True(t, exited.Load(),
		"Shutdown must JOIN the reap goroutine, not merely schedule it: its onExit is the only thing that "+
			"records the CLI's death, and everything the caller does next (drain the aggregates, close the "+
			"databases) takes that write away")
}

func TestShutdown_RefusesNewSessions(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()

	eng.Shutdown()

	_, err := eng.CreateCommand(ctx, "chat-after-shutdown", t.TempDir(),
		[]string{"cat"}, os.Environ(), nil)
	assert.ErrorIs(t, err, terminal.ErrShuttingDown,
		"CreateCommand must refuse after Shutdown: a vendor CLI nothing is left to reap outlives the daemon")

	_, err = eng.Create(ctx, "chat-after-shutdown", t.TempDir(), nil)
	assert.ErrorIs(t, err, terminal.ErrShuttingDown,
		"Create must refuse after Shutdown for the same reason: the kill loop has already been and gone")
}

func TestShutdown_RefusesBeforeStartingTheChild(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	eng.Shutdown()

	missing := filepath.Join(t.TempDir(), "no-such-binary")
	_, err := eng.CreateCommand(ctx, "chat-refused", t.TempDir(), []string{missing}, os.Environ(), nil)
	require.ErrorIs(t, err, terminal.ErrShuttingDown, "CreateCommand must refuse before exec")

	_, err = eng.Create(ctx, "chat-refused", t.TempDir(), &domain.TerminalProfile{Shell: missing})
	require.ErrorIs(t, err, terminal.ErrShuttingDown, "Create must refuse before exec")
}

func TestShutdown_RefusedRestoreKeepsPersistedState(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	const sid = "ph-refused-restore"
	scrollback := []byte("work the user has not read yet\r\n")
	require.NoError(t, eng.LoadPlaceholder(ctx, terminal.SessionMeta{
		SessionID: sid,
		ChatID:    "chat-refused-restore",
		CWD:       store.dir,
		Shell:     "/bin/sh",
		State:     "suspended",
	}, scrollback))
	require.NoError(t, os.WriteFile(filepath.Join(store.dir, sid+".buf"), scrollback, 0o644))

	terminal.BeginDrainForTest(eng)

	err := eng.Attach(ctx, sid, newMockConn())
	require.ErrorIs(t, err, terminal.ErrShuttingDown)

	assert.True(t, bufExists(store.dir, sid),
		"a restore refused by the shutdown must leave the scrollback on disk: the session is perfectly "+
			"restorable, we are simply not birthing PTYs any more — deleting it here would destroy the state "+
			"the graceful shutdown had just persisted for the next boot")
	assert.False(t, store.hasDeleted(sid),
		"a restore refused by the shutdown must not delete the session's meta row: the next daemon start "+
			"reloads the placeholder from it")
}
