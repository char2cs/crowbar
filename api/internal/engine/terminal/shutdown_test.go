package terminal_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// TestShutdown_FlushPersistNoBufDelete is the TDD spec test for Phase 3 graceful
// shutdown: every live session must be flushed to disk, saved with
// state="suspended", and NOT deleted, so a fresh engine can restore them on
// the next daemon start.
//
// Sequence:
//  1. Create N live sessions with a wired metaStore.
//  2. Wait for each session to emit output (confirms PTY is live).
//  3. Call Shutdown and wait for it to return within a deadline.
//  4. Assert .buf exists for each session.
//  5. Assert meta was saved with state="suspended".
//  6. Assert Delete was NOT called for any session.
//  7. Construct a fresh engine + LoadPlaceholder and assert sessions come back
//     as suspended placeholders.
func TestShutdown_FlushPersistNoBufDelete(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	const N = 3
	sids := make([]string, N)
	for i := range sids {
		sid, err := eng.Create(ctx, "ws-graceful-shutdown", store.dir, nil)
		require.NoError(t, err)
		sids[i] = sid
	}

	// Wait for each session to produce output — confirms the PTY is live and
	// the ring buffer has content that Shutdown can flush.
	for _, sid := range sids {
		waitForOutput(t, eng, sid, 10*time.Second)
	}

	// Shutdown must return promptly (maintenance goroutine closes the stop channel).
	shutdownDone := make(chan struct{})
	go func() {
		eng.Shutdown()
		close(shutdownDone)
	}()
	select {
	case <-shutdownDone:
	case <-time.After(15 * time.Second):
		t.Fatal("Shutdown did not return within 15 s")
	}

	// Brief window for any reapOnDone goroutines to complete their early-return
	// path (suspending=true prevents them from deleting anything).
	time.Sleep(300 * time.Millisecond)

	for _, sid := range sids {
		// 1. Scrollback buf must exist on disk (written by Shutdown before Kill).
		assert.True(t, bufExists(store.dir, sid),
			".buf must exist after Shutdown for sid=%s", sid)

		// 2. Meta must have been saved with state="suspended".
		assert.True(t, store.hasSavedWithState(sid, "suspended"),
			"meta must be saved with state=suspended after Shutdown for sid=%s", sid)

		// 3. Delete must NOT have been called — scrollback is preserved for restart.
		assert.False(t, store.hasDeleted(sid),
			"meta Delete must NOT be called during Shutdown for sid=%s", sid)
	}

	// 4. A fresh engine can reload sessions via LoadPlaceholder (simulating
	//    RestorePersistedSessions at daemon start).
	eng2 := terminal.New()
	terminal.StopMaintenanceForTest(eng2)
	for _, sid := range sids {
		loadErr := eng2.LoadPlaceholder(ctx, terminal.SessionMeta{
			SessionID:   sid,
			WorkspaceID: "ws-graceful-shutdown",
			CWD:         store.dir,
			Shell:       "/bin/sh",
			State:       "suspended",
		}, nil)
		require.NoError(t, loadErr)
		assert.True(t, eng2.SessionExists(ctx, sid),
			"session must be present in fresh engine after LoadPlaceholder (sid=%s)", sid)
		state, ok := eng2.StateOf(sid)
		assert.True(t, ok)
		assert.Equal(t, "suspended", state,
			"session state must be 'suspended' in fresh engine (sid=%s)", sid)
	}

	// Cleanup.
	for _, sid := range sids {
		_ = eng2.Kill(ctx, sid)
	}
	eng2.Shutdown()
}

// TestShutdown_NoDeleteForPlaceholder verifies that a placeholder session
// (already suspended, no live PTY) is NOT deleted during Shutdown. Its .buf
// and meta row must be preserved for restart-restore.
func TestShutdown_NoDeleteForPlaceholder(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	const sid = "ph-graceful-shutdown"
	scrollback := []byte("old prompt output\r\n")

	require.NoError(t, eng.LoadPlaceholder(ctx, terminal.SessionMeta{
		SessionID:   sid,
		WorkspaceID: "ws-ph-shutdown",
		CWD:         store.dir,
		Shell:       "/bin/sh",
		State:       "suspended",
	}, scrollback))

	// Pre-write a .buf to simulate a session that was flushed on a prior run.
	bufPath := filepath.Join(store.dir, sid+".buf")
	require.NoError(t, os.WriteFile(bufPath, scrollback, 0o644))

	eng.Shutdown()

	// .buf must survive Shutdown (not deleted).
	assert.True(t, bufExists(store.dir, sid),
		".buf must NOT be deleted for a placeholder session during Shutdown")

	// Delete must NOT have been called.
	assert.False(t, store.hasDeleted(sid),
		"Delete must NOT be called for a placeholder session during Shutdown")
}
