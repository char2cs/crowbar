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
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)

	// Each session is blocked on until its shell has printed its prompt — which confirms
	// the PTY is live and the screen model has content for Shutdown to flush, and does so by
	// observing the shell rather than by assuming 10 s is enough for it to have spoken.
	const N = 3
	sids := make([]string, N)
	for i := range sids {
		sids[i] = newReadyShell(t, eng, "chat-graceful-shutdown", store.dir)
	}

	// Shutdown is called directly, on this goroutine. It joins the maintenance sweep and
	// every reaper before returning (see the <-reaped at the end of Shutdown), so it
	// converges on the work itself; there is nothing here for a 15-second deadline to add
	// except a second, weaker definition of "too slow". If it ever fails to return, that is
	// a hang, and `go test -timeout` reports it with the blocked stack.
	eng.Shutdown()

	// Shutdown persists every live session synchronously — WriteBuf + saveMeta
	// (state="suspended"), both under the per-session lock — BEFORE it returns, and
	// marks each suspending so reapOnDone early-returns without deleting. Because
	// Shutdown has already returned (joined above), the positive state is durable,
	// so assert it directly.
	for _, sid := range sids {
		// 1. Scrollback buf must exist on disk (written by Shutdown before Kill).
		assert.True(t, bufExists(store.dir, sid),
			".buf must exist after Shutdown for sid=%s", sid)

		// 2. Meta must have been saved with state="suspended".
		assert.True(t, store.hasSavedWithState(sid, "suspended"),
			"meta must be saved with state=suspended after Shutdown for sid=%s", sid)
	}

	// 3. Delete must NEVER be called — scrollback is preserved for restart. The racing
	// reapOnDone goroutine (which must early-return on the suspending flag rather than
	// deleting) has ALREADY RUN TO COMPLETION: Shutdown ends by joining every reaper
	// (<-reaped), so the set of Delete calls is final and closed the moment it returns.
	//
	// So this is a plain read, not a 300 ms require.Never window. The window was trying to
	// out-wait goroutines that Shutdown provably joins — which made it both redundant and
	// WEAKER than the direct assertion: a Delete arriving at 301 ms would have satisfied it.
	for _, sid := range sids {
		assert.False(t, store.hasDeleted(sid),
			"meta Delete must NOT be called during Shutdown (sid=%s)", sid)
	}

	// 4. A fresh engine can reload sessions via LoadPlaceholder (simulating
	//    RestorePersistedSessions at daemon start).
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
		SessionID: sid,
		ChatID:    "chat-ph-shutdown",
		CWD:       store.dir,
		Shell:     "/bin/sh",
		State:     "suspended",
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

// TestShutdown_JoinsExitCallbacks pins the contract the daemon's whole graceful
// teardown now rests on: when Shutdown returns, every onExit callback the engine
// owed has ALREADY RUN.
//
// It is not bookkeeping. onExit is where a dying vendor CLI's death gets recorded —
// the agent usecase Exits its runner and closes the turn the CLI abandoned — and it
// runs on the engine's own reap goroutine. A Shutdown that returns while those
// goroutines are still in flight hands the caller a false "done", and the caller
// (app.Container.Shutdown) goes on to drain the aggregates and close the databases
// those callbacks were about to write to. Observed in production:
//
//	WARN agent: close abandoned turn: get chat … err="sql: database is closed"
//
// The runner's Exit had committed; the turn's close had not — and with no live runner
// row left, no boot reconcile could ever repair it. The chat spun forever.
//
// The assertion is made with a plain read, immediately after Shutdown returns. There
// is deliberately no channel wait, no Eventually and no sleep: waiting FOR the
// callback would test that it eventually runs (which it always did), when what must
// be proven is that it has already run by the time Shutdown hands back control.
func TestShutdown_JoinsExitCallbacks(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()

	// `cat` holds its PTY open exactly like a real vendor CLI, so this session is
	// still alive when the shutdown comes for it — the mid-turn shape.
	var exited atomic.Bool
	id, err := eng.CreateCommand(ctx, "chat-exit-drain", t.TempDir(),
		[]string{"cat"}, os.Environ(), func() { exited.Store(true) })
	require.NoError(t, err)
	require.True(t, eng.SessionExists(ctx, id))

	eng.Shutdown()

	assert.True(t, exited.Load(),
		"Shutdown must JOIN the reap goroutine, not merely schedule it: its onExit is the only thing that "+
			"records the CLI's death, and everything the caller does next (drain the aggregates, close the "+
			"databases) takes that write away")
}

// TestShutdown_RefusesNewSessions guards the other half of the drain: once Shutdown
// has begun, the engine births nothing.
//
// A session created after the kill loop has walked the registry is a PTY with no
// reaper — its process outlives the daemon and its onExit fires into a torn-down app
// (or never fires at all). It is also what would make the drain non-convergent, so
// refusing is what lets Shutdown's join be a guarantee rather than a hope. The one
// real caller that can still arrive this late is a hijacked WebSocket driving
// Attach→restore after the HTTP server has stopped accepting.
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

// TestShutdown_RefusedRestoreKeepsPersistedState is the guard for the way the
// refusal above could have gone wrong: a restore that is turned away because the
// engine is shutting down must not DESTROY the session it turned away.
//
// The restore path treats a failed spawn as proof that the session is unrestorable
// (the shell binary is gone, the cwd is gone) and drops it: registry entry removed,
// .buf and meta row DELETED, "ended" fired so the FE drops the tab. That verdict is
// right for a broken session and catastrophic for a healthy one — and "we are not
// birthing sessions any more" is not a fact about the session at all. Applied to a
// shutdown refusal it would delete the user's scrollback ON THE WAY OUT of the
// daemon, erasing the exact state the graceful shutdown had just persisted for the
// next boot. Reachable in production: a hijacked WebSocket can still drive
// Attach->restore after the HTTP server has stopped accepting.
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

	// The engine begins draining. The placeholder is still registered (Shutdown's kill
	// loop has not reached it), so a late Attach — a hijacked WebSocket can still drive
	// one after the HTTP server has stopped accepting — reaches the restore path and is
	// refused there. This is the window the refusal has to be right in; once Shutdown
	// has RETURNED, the placeholder is deregistered and Attach never gets that far.
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
