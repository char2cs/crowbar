package terminal

// Internal tests that need direct access to unexported types to cover
// defensive code paths unreachable from the external test package.

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
)

// dropMetaStore is a minimal SessionMetaStore for the restore-failure tests in
// the internal package. It records Delete calls and resolves a fixed dir.
type dropMetaStore struct {
	mu      sync.Mutex
	dir     string
	deleted map[string]struct{}
}

func (d *dropMetaStore) Save(_ context.Context, _ SessionMeta) error { return nil }

func (d *dropMetaStore) Delete(_ context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.deleted == nil {
		d.deleted = make(map[string]struct{})
	}
	d.deleted[id] = struct{}{}
	return nil
}

func (d *dropMetaStore) StorageDir(_ context.Context, _ string) (string, error) {
	return d.dir, nil
}

func (d *dropMetaStore) List(_ context.Context) ([]domain.TerminalSession, error) {
	return nil, nil
}

func (d *dropMetaStore) wasDeleted(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.deleted[id]
	return ok
}

// deadConn is a minimal WSConn that closes immediately on ReadMessage.
type deadConn struct{}

func (d *deadConn) WriteMessage(_ int, _ []byte) error { return nil }
func (d *deadConn) ReadMessage() (int, []byte, error)  { return 0, nil, io.EOF }
func (d *deadConn) Close() error                       { return nil }

// TestAttach_PlaceholderWithBadShell_ReturnsError covers the restore error path:
// the session IS in the registry as a placeholder, but the stored shell binary
// does not exist, so restore (spawn) fails and Attach must return an error.
// (Previously this test injected a killed session and expected s.Attach() to error
// because done was closed. With restore-aware Attach, a not-live session triggers
// restore instead, so we exercise the restore-failure path instead.)
func TestAttach_PlaceholderWithBadShell_ReturnsError(t *testing.T) {
	// Create a placeholder with an invalid shell so spawn fails during restore.
	ph := session.NewPlaceholder("ph-bad", "/nonexistent/shell/binary", t.TempDir(), "", nil)

	eng := New().(*terminalEngine)
	StopMaintenanceForTest(eng)
	eng.reg.Add("ph-bad", "ws-bad", ph)

	err := eng.Attach(context.Background(), "ph-bad", &deadConn{})
	assert.Error(t, err, "Attach on a placeholder with a bad shell must return an error")
}

// TestFireEnded_NilCallback covers the no-callback short-circuit: fireEnded must
// be a no-op (and must not record the session as ended) when no callback is set.
func TestFireEnded_NilCallback(t *testing.T) {
	eng := New().(*terminalEngine)
	StopMaintenanceForTest(eng)
	eng.fireEnded(context.Background(), "ws", "s1", 0)
	assert.NotContains(t, eng.endedOnce, "s1")
}

// TestFireEnded_FiresExactlyOnce covers the duplicate guard: a second fireEnded
// for the same session id must not re-invoke the callback.
func TestFireEnded_FiresExactlyOnce(t *testing.T) {
	eng := New().(*terminalEngine)
	StopMaintenanceForTest(eng)
	var calls int
	eng.OnSessionEnded(func(_ context.Context, _, _ string, _ int) { calls++ })

	eng.fireEnded(context.Background(), "ws", "s1", 0)
	eng.fireEnded(context.Background(), "ws", "s1", 0)

	assert.Equal(t, 1, calls)
}

// TestFireState_NilCallback covers the nil-safe guard: fireState must be a
// no-op when no OnSessionState callback is registered.
func TestFireState_NilCallback(t *testing.T) {
	eng := New().(*terminalEngine)
	StopMaintenanceForTest(eng)
	// Must not panic.
	eng.fireState(context.Background(), "ws", "s1", "detached")
}

// TestFireState_FiresCallback verifies fireState invokes the registered callback
// with the correct arguments.
func TestFireState_FiresCallback(t *testing.T) {
	eng := New().(*terminalEngine)
	StopMaintenanceForTest(eng)
	type call struct {
		ws    string
		sid   string
		state string
	}
	ch := make(chan call, 2)
	eng.OnSessionState(func(_ context.Context, ws, sid, state string) {
		ch <- call{ws, sid, state}
	})

	eng.fireState(context.Background(), "ws-1", "s-1", "detached")
	eng.fireState(context.Background(), "ws-1", "s-1", "suspended")

	got1 := <-ch
	assert.Equal(t, call{"ws-1", "s-1", "detached"}, got1)
	got2 := <-ch
	assert.Equal(t, call{"ws-1", "s-1", "suspended"}, got2)
}

// TestRestore_CWDFallback_SpawnsInHomeWhenSavedDirGone verifies that when the
// saved working directory no longer exists, restore falls back to a valid
// directory and the spawn SUCCEEDS instead of stranding the placeholder.
func TestRestore_CWDFallback_SpawnsInHomeWhenSavedDirGone(t *testing.T) {
	eng := New().(*terminalEngine)
	StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ctx := context.Background()

	goneDir := filepath.Join(t.TempDir(), "deleted-worktree") // never created
	ph := session.NewPlaceholder("ph-cwd", "/bin/sh", goneDir, "", nil)
	eng.reg.Add("ph-cwd", "ws-1", ph)

	err := eng.restore(ctx, "ph-cwd")
	require.NoError(t, err, "restore must succeed via the CWD fallback")

	s, ok := eng.reg.Get("ph-cwd")
	require.True(t, ok, "session must remain in the registry")
	require.True(t, s.IsLive(), "restored session must be live")
	t.Cleanup(func() { _ = eng.Kill(ctx, "ph-cwd") })
}

// TestRestore_BadShell_DropsPlaceholder verifies that when the spawn fails even
// after the CWD fallback (the shell binary is gone), restore does NOT leave the
// placeholder behind: it removes the registry entry, deletes the persisted .buf
// and meta row, and fires ended — so Attach can never loop forever on a doomed
// restore.
func TestRestore_BadShell_DropsPlaceholder(t *testing.T) {
	eng := New().(*terminalEngine)
	StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	ctx := context.Background()

	store := &dropMetaStore{dir: t.TempDir()}
	eng.SetMetaStore(store)

	var ended []string
	eng.OnSessionEnded(func(_ context.Context, _, sid string, _ int) {
		ended = append(ended, sid)
	})

	// Persisted .buf on disk to prove the drop deletes it.
	require.NoError(t, os.WriteFile(filepath.Join(store.dir, "ph-bad.buf"), []byte("x"), 0o644))

	realCWD := t.TempDir() // a valid dir, so the failure is the shell, not the cwd
	ph := session.NewPlaceholder("ph-bad", "/nonexistent/shell/binary", realCWD, "", nil)
	eng.reg.Add("ph-bad", "ws-1", ph)

	err := eng.restore(ctx, "ph-bad")
	require.Error(t, err, "restore must return an error when the shell binary is missing")

	_, ok := eng.reg.Get("ph-bad")
	assert.False(t, ok, "placeholder must be removed from the registry (no infinite Attach retry)")
	_, statErr := os.Stat(filepath.Join(store.dir, "ph-bad.buf"))
	assert.True(t, os.IsNotExist(statErr), "persisted .buf must be deleted")
	assert.True(t, store.wasDeleted("ph-bad"), "meta row must be deleted")
	assert.Contains(t, ended, "ph-bad", "ended must fire so the FE drops the dead tab")
}
