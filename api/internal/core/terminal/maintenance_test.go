package terminal_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

// ---------------------------------------------------------------------------
// helpers shared by this file
// ---------------------------------------------------------------------------

// countSavedForSession returns how many times the meta store recorded a Save
// for the given session ID.
func countSavedForSession(store *fakeMetaStore, sid string) int {
	store.mu.Lock()
	defer store.mu.Unlock()
	n := 0
	for _, m := range store.saved {
		if m.SessionID == sid {
			n++
		}
	}
	return n
}

// The clock-based readiness helpers that used to live here — waitIdle, waitNotIdle,
// waitForModelOutput and waitForSettled — are gone. Each was a guess dressed up as a wait:
//
//   - waitIdle polled TIOCGPGRP and returned as soon as the shell was the foreground
//     process group, which is BEFORE it has printed its prompt.
//   - waitForSettled tried to patch that hole by declaring the shell "done" after 250 ms of
//     no growth, i.e. by inferring quiescence from silence. A shell is under no obligation
//     to be prompt about being quiet, and when a straggler prompt chunk landed after the
//     window, dirty flipped back on and TestMaintenance_CadenceFlush failed.
//
// They are replaced by waitPrompt / runShell / startForeground (shellsync_test.go), which
// block on the shell's own protocol — its prompt — and on the pump's own progress signal.
// See that file for the reasoning.

// ---------------------------------------------------------------------------
// TestPtyEnv_DefaultLocale
// ---------------------------------------------------------------------------

// TestPtyEnv_DefaultLocale verifies ptyEnv's UTF-8-locale defaulting (via the
// pure defaultLocale decision it delegates to): a GUI/launchd-launched daemon
// that inherited NO locale gets a platform-appropriate UTF-8 LANG (en_US.UTF-8
// on darwin, C.UTF-8 elsewhere), while ANY explicitly-set locale var — LANG,
// LC_ALL or LC_CTYPE — is left untouched. This is the unit guard for the
// no-LANG glyph-corruption fix; the integration suite proves it end-to-end.
func TestPtyEnv_DefaultLocale(t *testing.T) {
	cases := []struct {
		name string
		base []string
		goos string
		want string
	}{
		{"unset-darwin-defaults-en_US", []string{"TERM=xterm-256color", "PATH=/usr/bin"}, "darwin", "en_US.UTF-8"},
		{"unset-linux-defaults-C_UTF8", []string{"TERM=xterm-256color", "PATH=/usr/bin"}, "linux", "C.UTF-8"},
		{"lang-set-untouched-darwin", []string{"LANG=en_GB.ISO-8859-1"}, "darwin", ""},
		{"lang-set-untouched-linux", []string{"LANG=en_GB.ISO-8859-1"}, "linux", ""},
		{"lc-all-set-untouched", []string{"LC_ALL=fr_FR.UTF-8"}, "darwin", ""},
		{"lc-ctype-set-untouched", []string{"LC_CTYPE=de_DE.UTF-8"}, "linux", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, terminal.DefaultLocaleForTest(tc.base, tc.goos))
		})
	}
}

// ---------------------------------------------------------------------------
// TestMaintenance_CadenceFlush
// ---------------------------------------------------------------------------

// TestMaintenance_CadenceFlush verifies that runMaintenanceOnce flushes the .buf
// and meta for a dirty session, then does NOT flush again when the session
// produces no new output. Dirty-gating now flows through Snapshot()'s
// (blob, changed) return: the first flush sees changed=true and persists, which
// clears the dirty bit under the model lock, so the next Snapshot() reports
// changed=false and the maintenance run skips the write.
func TestMaintenance_CadenceFlush(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng) // prevent ticker from racing with RunMaintenanceOnceForTest
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	// Block until the shell has printed its prompt. That — not a duration, and not the
	// foreground-pgroup check, which goes true BEFORE the prompt is written — is the point
	// at which the shell has said everything it is going to say and is blocked on read(2).
	// The session is therefore dirty (the prompt went through pumpStep) and, crucially,
	// NOTHING further is in flight, which is the precondition the "no second flush"
	// assertion below actually depends on.
	sid := newReadyShell(t, eng, "ws-flush", dir)

	// First maintenance run — dirty=true → flush happens.
	savesBefore := countSavedForSession(store, sid)
	terminal.RunMaintenanceOnceForTest(eng, ctx)

	// .buf must now exist and meta must have been saved.
	assert.True(t, bufExists(store.dir, sid), ".buf must be written on first cadence flush")
	savesAfter := countSavedForSession(store, sid)
	assert.Greater(t, savesAfter, savesBefore, "meta must be saved on first cadence flush")
	savesQuiet := countSavedForSession(store, sid)

	// The first run consumed the dirty bit via Snapshot(). No PTY byte has been written
	// since (the shell is parked at its prompt), so nothing can have set it again — a
	// second run must find changed=false and skip the write. Previously this needed a
	// speculative "drain the stragglers" flush plus two settle windows to paper over the
	// fact that the test could not actually tell whether the shell had finished talking.
	// Now it can, so the assertion is made directly.
	terminal.RunMaintenanceOnceForTest(eng, ctx)
	assert.Equal(t, savesQuiet, countSavedForSession(store, sid),
		"no new meta save when session has no new output (dirty=false)")

	// Conversely, prove the gate is a real dirty gate and not just a dead code path: give
	// the shell something to say, wait for it to finish saying it, and the very next run
	// must flush again.
	runShell(t, eng, sid, "echo dirty-again")
	terminal.RunMaintenanceOnceForTest(eng, ctx)
	assert.Greater(t, countSavedForSession(store, sid), savesQuiet,
		"a session with new output since the last flush must be flushed again")

	require.NoError(t, eng.Kill(ctx, sid))
}

// ---------------------------------------------------------------------------
// TestMaintenance_SoftLimit
// ---------------------------------------------------------------------------

// TestMaintenance_SoftLimit verifies that with softLimitPerWorkspace=2 and four
// idle detached sessions in one workspace, runMaintenanceOnce suspends the two
// oldest (by lastActive) and leaves the two newest live.
func TestMaintenance_SoftLimit(t *testing.T) {
	pinShell(t)

	restore := terminal.SetSoftLimitPerWorkspaceForTest(2)
	defer restore()

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng) // prevent ticker from racing with limit-var writes
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	// Create 4 sessions in the same workspace, each blocked on until it is parked at its
	// prompt — the state the sweep's idle gate is meant to see.
	var sids [4]string
	for i := range sids {
		sids[i] = newReadyShell(t, eng, "ws-softlimit", dir)
	}

	// Assign staggered lastActive times: sids[0] oldest, sids[3] newest.
	base := time.Now().Add(-10 * time.Minute)
	for i, sid := range sids {
		terminal.SetLastActiveForTest(eng, sid, base.Add(time.Duration(i)*time.Minute))
	}

	// Run maintenance — should suspend sids[0] and sids[1] (2 oldest).
	terminal.RunMaintenanceOnceForTest(eng, ctx)

	// No wait here, and none is needed: runMaintenanceOnce calls Suspend SYNCHRONOUSLY, and
	// Suspend has persisted the "suspended" meta and swapped in the placeholder before it
	// returns. The effect is therefore already complete the moment the sweep returns, so
	// the assertions below are made directly. (An Eventually here would not be a wait for
	// anything — it would merely give a genuine ordering regression 10 seconds of retries
	// in which to look like a pass.)
	assert.True(t, store.hasSavedWithState(sids[0], "suspended"),
		"oldest session must be suspended by soft limit")
	assert.True(t, store.hasSavedWithState(sids[1], "suspended"),
		"second-oldest session must be suspended by soft limit")

	// sids[2] and sids[3] must still be live.
	assert.True(t, eng.SessionExists(ctx, sids[2]), "third session must still exist")
	assert.True(t, eng.SessionExists(ctx, sids[3]), "fourth (newest) session must still exist")

	// Verify sids[2] and sids[3] are NOT placeholders (still writable).
	assert.NoError(t, eng.Write(ctx, sids[2], []byte("echo still-alive\n")),
		"third session must still be writable (live)")
	assert.NoError(t, eng.Write(ctx, sids[3], []byte("echo still-alive\n")),
		"fourth session must still be writable (live)")

	// Cleanup
	for _, sid := range sids {
		_ = eng.Kill(ctx, sid)
	}
}

// ---------------------------------------------------------------------------
// TestMaintenance_RunningNeverIdleSuspended
// ---------------------------------------------------------------------------

// TestMaintenance_RunningNeverIdleSuspended verifies that the soft-limit path
// never suspends a detached session that has a running foreground child, even
// when the workspace is over the limit.
func TestMaintenance_RunningNeverIdleSuspended(t *testing.T) {
	pinShell(t)

	restore := terminal.SetSoftLimitPerWorkspaceForTest(1)
	defer restore()

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng) // prevent ticker from racing with limit-var writes
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	// Create an idle session (will be over limit) — parked at its prompt, so TIOCGPGRP
	// reports the shell itself as the foreground group: genuinely idle.
	sidIdle := newReadyShell(t, eng, "ws-running", dir)

	// Create a running session and give it a foreground child. startForeground returns only
	// once the kernel already reports the CHILD as the terminal's foreground process group,
	// which is the exact condition the sweep's idle gate tests — so there is nothing left to
	// "settle" and no transient window to debounce.
	sidRunning := newReadyShell(t, eng, "ws-running", dir)
	startForeground(t, eng, sidRunning)

	// Assign lastActive: running session is "older" so it would be first candidate
	// if idle — but it's not idle, so it must be skipped.
	base := time.Now().Add(-10 * time.Minute)
	terminal.SetLastActiveForTest(eng, sidRunning, base)
	terminal.SetLastActiveForTest(eng, sidIdle, base.Add(time.Minute))

	// Workspace has 2 detached sessions > limit of 1: the running one must be skipped, the
	// idle one suspended. ONE pass is enough and is asserted as such. This used to be a
	// retry loop, on the theory that the idle check could "transiently read non-idle under
	// load and skip a pass" — but that transient was an artefact of starting the sweep
	// before the shell had actually reached its prompt. Both sessions are now in a known,
	// kernel-confirmed state before the sweep runs, so a missed suspend is a real bug and
	// must fail rather than be retried until it passes.
	terminal.RunMaintenanceOnceForTest(eng, ctx)
	assert.True(t, store.hasSavedWithState(sidIdle, "suspended"),
		"the idle over-limit session must be suspended in a single sweep")

	// sidRunning must still be live and writable.
	assert.NoError(t, eng.Write(ctx, sidRunning, []byte("echo running\n")),
		"running session must NOT be suspended by the soft-limit (idle-gated) path")

	_ = eng.Kill(ctx, sidIdle)
	_ = eng.Kill(ctx, sidRunning)
}

// ---------------------------------------------------------------------------
// TestMaintenance_GlobalForceLastResort
// ---------------------------------------------------------------------------

// TestMaintenance_GlobalForceLastResort verifies the last-resort path: when the
// global model-byte ceiling is exceeded and both detached sessions have a running
// foreground child (neither idle), the global ceiling exhausts idle candidates
// (none) and then force-suspends the oldest detached session. The suspended
// session's .buf must contain the resource notice.
//
// The trigger is the BYTE ceiling: two live sessions pin two full-budget models;
// the ceiling is set between one and two models so force-suspending the oldest
// (which swaps its live model for a tiny placeholder blob) brings us back under,
// leaving the placeholder intact rather than evicting it. The count ceiling is
// left at its default so the surviving placeholder is not LRU-evicted.
//
// The ceiling is derived from the engine's own model-byte accounting (two live
// ModelBytes) rather than hardcoded, so it tracks the default model size automatically.
func TestMaintenance_GlobalForceLastResort(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng) // prevent ticker from racing with limit-var writes
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	// Create two sessions, each blocked on until it is parked at its prompt.
	sid1 := newReadyShell(t, eng, "ws-force", dir)
	sid2 := newReadyShell(t, eng, "ws-force", dir)

	// Two live full-budget models are now accounted; set the ceiling at 75% of
	// that (between one and two models) so the global byte ceiling fires and a
	// single force-suspend brings us back under.
	_, _, _, modelBytes, _, _ := eng.Stats()
	restoreBytes := terminal.SetMaxTotalModelBytesForTest(modelBytes * 3 / 4)
	defer restoreBytes()

	// Give both a running foreground child. startForeground blocks until TIOCGPGRP already
	// reports the child's process group, so both sessions are non-idle by the sweep's own
	// measure before it runs — no idle candidates, which is the precondition this test needs
	// in order to reach the last-resort force-suspend at all.
	startForeground(t, eng, sid1)
	startForeground(t, eng, sid2)

	// sid1 is "older" — it should be force-suspended first.
	base := time.Now().Add(-10 * time.Minute)
	terminal.SetLastActiveForTest(eng, sid1, base)
	terminal.SetLastActiveForTest(eng, sid2, base.Add(time.Minute))

	// Run maintenance: 2 live models (512 KB) > 384 KB ceiling → global ceiling fires.
	// No idle candidates → last-resort force-suspend of sid1 (oldest).
	terminal.RunMaintenanceOnceForTest(eng, ctx)

	// The force-suspend runs synchronously inside the sweep (WriteBuf then saveMeta
	// "suspended" before it returns), so the moment the sweep returns the .buf is already on
	// disk and the row already recorded. Asserted directly — there is no later moment at
	// which this could become true, so polling for one would only mask a regression.
	assert.True(t, store.hasSavedWithState(sid1, "suspended"),
		"oldest session must be force-suspended by global ceiling last resort")

	// The .buf for sid1 must contain the resource notice.
	bufData, readErr := os.ReadFile(filepath.Join(store.dir, sid1+".buf"))
	require.NoError(t, readErr, ".buf must exist after force-suspend")
	assert.Contains(t, string(bufData), "suspended to free resources",
		"force-suspend buf must contain the resource-freed notice")

	// sid2 must still be writable (live).
	assert.NoError(t, eng.Write(ctx, sid2, []byte("echo alive\n")),
		"sid2 must still be live after global force-suspend of sid1")

	_ = eng.Kill(ctx, sid1)
	_ = eng.Kill(ctx, sid2)
}

// ---------------------------------------------------------------------------
// TestMaintenance_CommandSessionsNeverSuspended
// ---------------------------------------------------------------------------

// TestMaintenance_CommandSessionsNeverSuspended guards the fatal agentic-CLI bug: a
// command session (spawned via CreateCommand, e.g. an agentic vendor CLI) must never be
// idle- or force-suspended by the maintenance sweep, even when it is the sole cause of
// blowing through both the per-workspace soft limit and the global session-count
// ceiling. Suspend would tear down the PTY (killing the vendor process outright) and a
// subsequent restore cannot bring it back (it would exec.Command the joined argv string
// instead of the original binary) — so these sessions must be entirely invisible to the
// sweep's candidate-collection loops.
func TestMaintenance_CommandSessionsNeverSuspended(t *testing.T) {
	restoreSoft := terminal.SetSoftLimitPerWorkspaceForTest(0)
	defer restoreSoft()
	restoreCeiling := terminal.SetMaxTotalSessionsForTest(1)
	defer restoreCeiling()

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng) // prevent ticker from racing with limit-var writes
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	// Two long-running, detached command sessions in one workspace: soft limit is 0
	// (any detached session is "excess") and the global ceiling is 1 (two sessions is
	// already over), so both phases would fire on ordinary shell sessions.
	sid1, err := eng.CreateCommand(ctx, "ws-cmd-ceiling", dir,
		[]string{"/bin/sh", "-c", "sleep 9999"}, os.Environ(), nil)
	require.NoError(t, err)
	sid2, err := eng.CreateCommand(ctx, "ws-cmd-ceiling", dir,
		[]string{"/bin/sh", "-c", "sleep 9999"}, os.Environ(), nil)
	require.NoError(t, err)

	// Oldest-first ordering would normally pick sid1 first for eviction; stagger
	// lastActive so the test would fail loudly (not by accident) if the guard broke.
	base := time.Now().Add(-10 * time.Minute)
	terminal.SetLastActiveForTest(eng, sid1, base)
	terminal.SetLastActiveForTest(eng, sid2, base.Add(time.Minute))

	// The sweep is synchronous end to end: had it decided to suspend these sessions, the
	// suspend (and its "suspended" meta row) would already have happened by the time
	// RunMaintenanceOnceForTest returned. So the guard can be asserted immediately.
	//
	// The 200 ms sleep that used to sit here was trying to "give a suspend a chance to
	// happen" before declaring that it hadn't — a negative assertion fenced by a guess.
	// It proved nothing: a slow-enough machine would have passed the test no matter how
	// broken the guard was.
	terminal.RunMaintenanceOnceForTest(eng, ctx)

	for _, sid := range []string{sid1, sid2} {
		assert.True(t, eng.SessionExists(ctx, sid), "command session %s must still exist", sid)
		state, ok := eng.StateOf(sid)
		require.True(t, ok, "command session %s must still be registered", sid)
		assert.NotEqual(t, "suspended", state, "command session %s must never be suspended", sid)
		assert.False(t, store.hasSavedWithState(sid, "suspended"),
			"command session %s must never be persisted as suspended", sid)
		assert.NoError(t, eng.Write(ctx, sid, []byte("echo still-alive\n")),
			"command session %s must still be live/writable", sid)
	}

	_ = eng.Kill(ctx, sid1)
	_ = eng.Kill(ctx, sid2)
}

// ---------------------------------------------------------------------------
// TestMaintenance_ShutdownStopsGoroutine
// ---------------------------------------------------------------------------

// TestMaintenance_ShutdownStopsGoroutine verifies that Shutdown closes the stop
// channel (goroutine exits cleanly) and that a subsequent double-Shutdown and
// runMaintenanceOnce call do not panic.
func TestMaintenance_ShutdownStopsGoroutine(t *testing.T) {
	eng := terminal.New()
	// Note: we do NOT call StopMaintenanceForTest here because this test
	// specifically exercises that Shutdown() stops the maintenance goroutine.
	// However Shutdown() is called immediately so the goroutine window is tiny.
	ctx := context.Background()

	// Shutdown must return. Called directly: if it ever fails to return, that is a hang, and
	// `go test -timeout` reports it with a goroutine dump naming the exact blocked stack —
	// strictly more useful than a hand-rolled "did not return within 5 s" would be, and
	// without a 5-second guess about how long a shutdown is allowed to take.
	eng.Shutdown()

	// Double Shutdown must not panic (stopOnce guard).
	assert.NotPanics(t, func() { eng.Shutdown() })

	// runMaintenanceOnce on a shut-down engine must not panic.
	assert.NotPanics(t, func() {
		terminal.RunMaintenanceOnceForTest(eng, ctx)
	})
}

// ---------------------------------------------------------------------------
// TestEngine_Stats
// ---------------------------------------------------------------------------

// TestEngine_Stats verifies that Stats returns correct counts after session
// creates, suspensions, and kills.
func TestEngine_Stats(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	// No sessions yet.
	a, d, s, rb, deg, pp := eng.Stats()
	assert.Zero(t, a+d+s, "no sessions at start")
	assert.Zero(t, rb, "no model bytes at start")
	assert.Zero(t, deg, "a clean engine reports zero degraded sessions (§9.4)")
	assert.Zero(t, pp, "a clean engine reports zero parse panics (§9.4)")

	// Create one session — it starts detached (live, no clients) — and block until it is
	// parked at its prompt, so the idle gate Suspend consults has a settled answer.
	sid := newReadyShell(t, eng, "ws-stats", dir)

	_, det, _, modelB, deg2, pp2 := eng.Stats()
	assert.Equal(t, 1, det, "one detached session")
	assert.Greater(t, modelB, int64(0), "model bytes must be positive for a live session")
	assert.Zero(t, deg2, "a healthy live session reports not-degraded")
	assert.Zero(t, pp2, "a healthy live session reports zero parse panics")

	// Suspend it — must move to suspended. One call, asserted. This was a retry loop that
	// re-issued Suspend every 200 ms until it stuck, which quietly tolerated Suspend failing
	// its idle gate: with the shell now provably at its prompt, the first call must succeed,
	// and if it does not, that is a bug worth failing on rather than retrying past.
	require.NoError(t, eng.Suspend(ctx, sid))
	require.True(t, store.hasSavedWithState(sid, "suspended"),
		"Suspend persists the suspended meta before returning")

	_, detAfter, susp, _, _, _ := eng.Stats()
	assert.Zero(t, detAfter, "no detached sessions after suspend")
	assert.Equal(t, 1, susp, "one suspended session")

	_ = eng.Kill(ctx, sid)
}
