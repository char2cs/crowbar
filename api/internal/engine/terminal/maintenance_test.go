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

// waitIdle polls IsIdleForTest until it returns true or deadline.
func waitIdle(t *testing.T, eng terminal.Engine, id string, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for {
		if terminal.IsIdleForTest(eng, id) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("session %s did not become idle within %s", id, d)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// waitNotIdle polls IsIdleForTest until it returns false consistently (3 consecutive
// non-idle readings separated by 50 ms). A single non-idle reading is insufficient
// because TIOCGPGRP can return the shell's pgroup transiently before the foreground
// child (e.g. sleep) sets up its own process group; Getpgid errors also cause
// isIdleLocked to return false, making waitNotIdle exit too early.
func waitNotIdle(t *testing.T, eng terminal.Engine, id string, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	const required = 3
	confirmed := 0
	for {
		if !terminal.IsIdleForTest(eng, id) { //nolint:nestif // require N consecutive non-idle readings; the confirm/reset counter is the debounce this test needs
			confirmed++
			if confirmed >= required {
				return
			}
		} else {
			confirmed = 0
		}
		select {
		case <-deadline:
			t.Fatalf("session %s did not become non-idle (running) within %s", id, d)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// waitForModelOutput polls until the session's serialized model is non-empty (i.e.
// the shell has emitted at least one byte — typically the prompt). This is needed
// because IsIdle() returns true as soon as the shell is the foreground process
// group, which is BEFORE it writes the prompt bytes to the PTY. Without this
// extra wait the cadence-flush check (dirty=true) races with pumpStep.
func waitForModelOutput(t *testing.T, eng terminal.Engine, id string, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for {
		if terminal.SnapshotLenForTest(eng, id) > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("session %s did not emit any output within %s", id, d)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// waitForSettled polls until the session's serialized model has not grown for at
// least 5 consecutive 50 ms samples (250 ms of silence). Shell prompts arrive
// in multiple PTY chunks; waitForModelOutput returns on the first chunk, but dirty
// will still be set by later chunks. waitForSettled ensures the pump has fully
// drained the shell's initial output before the test triggers a maintenance run.
func waitForSettled(t *testing.T, eng terminal.Engine, id string, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	const stableNeeded = 5
	lastLen := -1
	stableCount := 0
	for {
		curLen := terminal.SnapshotLenForTest(eng, id)
		if curLen == lastLen { //nolint:nestif // settle detector: N consecutive equal-length samples; the count/reset branch is the debounce
			stableCount++
			if stableCount >= stableNeeded {
				return
			}
		} else {
			stableCount = 0
			lastLen = curLen
		}
		select {
		case <-deadline:
			t.Fatalf("session %s serialized model did not settle within %s", id, d)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

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
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng) // prevent ticker from racing with RunMaintenanceOnceForTest
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	sid, err := eng.Create(ctx, "ws-flush", dir, nil)
	require.NoError(t, err)

	// Wait for the shell to start and emit its prompt (sets dirty=true via pumpStep).
	// waitIdle returns as soon as the shell is the foreground process group (kernel
	// check), which can be BEFORE prompt bytes propagate through pumpStep. We also
	// wait until the serialized model has settled (no growth for 250 ms) to ensure all
	// prompt chunks have been processed by pumpStep and dirty=true is set.
	waitIdle(t, eng, sid, 10*time.Second)
	waitForSettled(t, eng, sid, 10*time.Second)

	// First maintenance run — dirty=true → flush happens.
	savesBefore := countSavedForSession(store, sid)
	terminal.RunMaintenanceOnceForTest(eng, ctx)

	// .buf must now exist and meta must have been saved.
	assert.True(t, bufExists(store.dir, sid), ".buf must be written on first cadence flush")
	savesAfter := countSavedForSession(store, sid)
	assert.Greater(t, savesAfter, savesBefore, "meta must be saved on first cadence flush")

	// Drain any straggler prompt output the shell may still emit (especially slow
	// under -race), and flush it, so the session reliably reaches dirty=false
	// BEFORE the assertion run. Without this, a late prompt chunk sets dirty=true
	// between the two runs and the "no flush when not dirty" assertion flakes.
	waitForSettled(t, eng, sid, 5*time.Second)
	terminal.RunMaintenanceOnceForTest(eng, ctx) // flush any stragglers, clearing dirty
	waitForSettled(t, eng, sid, 5*time.Second)
	savesQuiet := countSavedForSession(store, sid)

	// With dirty=false and the shell idle, a maintenance run must NOT flush again.
	terminal.RunMaintenanceOnceForTest(eng, ctx)
	assert.Equal(t, savesQuiet, countSavedForSession(store, sid),
		"no new meta save when session has no new output (dirty=false)")

	require.NoError(t, eng.Kill(ctx, sid))
}

// ---------------------------------------------------------------------------
// TestMaintenance_SoftLimit
// ---------------------------------------------------------------------------

// TestMaintenance_SoftLimit verifies that with softLimitPerWorkspace=2 and four
// idle detached sessions in one workspace, runMaintenanceOnce suspends the two
// oldest (by lastActive) and leaves the two newest live.
func TestMaintenance_SoftLimit(t *testing.T) {
	restore := terminal.SetSoftLimitPerWorkspaceForTest(2)
	defer restore()

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng) // prevent ticker from racing with limit-var writes
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	// Create 4 sessions in the same workspace.
	var sids [4]string
	for i := range sids {
		sid, err := eng.Create(ctx, "ws-softlimit", dir, nil)
		require.NoError(t, err)
		sids[i] = sid
	}

	// Wait for all four to be idle AND fully settled at their prompts.
	// waitIdle alone returns on the first true TIOCGPGRP reading, which can be
	// a transient idle moment between shell-init sub-processes. waitForSettled
	// ensures 250 ms of output silence, guaranteeing the shell is truly at
	// its prompt before we trigger the maintenance sweep.
	for _, sid := range sids {
		waitIdle(t, eng, sid, 15*time.Second)
		waitForSettled(t, eng, sid, 10*time.Second)
	}

	// Assign staggered lastActive times: sids[0] oldest, sids[3] newest.
	base := time.Now().Add(-10 * time.Minute)
	for i, sid := range sids {
		terminal.SetLastActiveForTest(eng, sid, base.Add(time.Duration(i)*time.Minute))
	}

	// Run maintenance — should suspend sids[0] and sids[1] (2 oldest).
	terminal.RunMaintenanceOnceForTest(eng, ctx)

	// runMaintenanceOnce calls Suspend synchronously, and Suspend persists the
	// "suspended" meta (and the placeholder swap) before returning, so the state
	// is already durable here. Wait on that real observable deterministically
	// rather than sleeping for the Kill goroutines.
	require.Eventually(t, func() bool {
		return store.hasSavedWithState(sids[0], "suspended") &&
			store.hasSavedWithState(sids[1], "suspended")
	}, 10*time.Second, 10*time.Millisecond,
		"two oldest sessions must be suspended by soft limit")

	// sids[0] and sids[1] must be suspended (placeholders).
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
	restore := terminal.SetSoftLimitPerWorkspaceForTest(1)
	defer restore()

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng) // prevent ticker from racing with limit-var writes
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	// These waits drive real shell subprocesses (spawn, prompt emission, fork/exec
	// of `sleep`). Under a saturated full-suite `-race` run the subprocess can be
	// starved well past a tight deadline (observed: waitNotIdle timing out at 10 s),
	// so the deadline is generous — a passing wait returns as soon as its condition
	// holds, so headroom never slows the common case. This engine's maintenance
	// ticker is stopped (StopMaintenanceForTest above), so a longer wait cannot race
	// a ticker firing. waitForSettled before the write ensures the shell is fully
	// initialised (250 ms of output silence) so it runs the sleep command promptly
	// once scheduled.
	const wait = 30 * time.Second

	// Create an idle session (will be over limit).
	sidIdle, err := eng.Create(ctx, "ws-running", dir, nil)
	require.NoError(t, err)
	waitIdle(t, eng, sidIdle, wait)
	waitForSettled(t, eng, sidIdle, wait)

	// Create a running session (foreground sleep).
	sidRunning, err := eng.Create(ctx, "ws-running", dir, nil)
	require.NoError(t, err)
	waitIdle(t, eng, sidRunning, wait)
	waitForSettled(t, eng, sidRunning, wait)
	require.NoError(t, eng.Write(ctx, sidRunning, []byte("sleep 9999\n")))
	waitNotIdle(t, eng, sidRunning, wait)

	// Assign lastActive: running session is "older" so it would be first candidate
	// if idle — but it's not idle, so it must be skipped.
	base := time.Now().Add(-10 * time.Minute)
	terminal.SetLastActiveForTest(eng, sidRunning, base)
	terminal.SetLastActiveForTest(eng, sidIdle, base.Add(time.Minute))

	// Run maintenance — workspace has 2 detached sessions > limit of 1.
	// The running one must be skipped; the idle one gets suspended.
	terminal.RunMaintenanceOnceForTest(eng, ctx)

	// Suspend runs synchronously inside the sweep and persists "suspended" before
	// returning; wait on that observable state rather than sleeping.
	require.Eventually(t, func() bool {
		return store.hasSavedWithState(sidIdle, "suspended")
	}, 10*time.Second, 10*time.Millisecond,
		"idle detached session must be suspended by soft limit")

	// sidIdle must be suspended.
	assert.True(t, store.hasSavedWithState(sidIdle, "suspended"),
		"idle detached session must be suspended by soft limit")

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
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng) // prevent ticker from racing with limit-var writes
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	// Create two sessions that will have running foreground children.
	sid1, err := eng.Create(ctx, "ws-force", dir, nil)
	require.NoError(t, err)
	sid2, err := eng.Create(ctx, "ws-force", dir, nil)
	require.NoError(t, err)

	// Two live full-budget models are now accounted; set the ceiling at 75% of
	// that (between one and two models) so the global byte ceiling fires and a
	// single force-suspend brings us back under.
	_, _, _, modelBytes, _, _ := eng.Stats()
	restoreBytes := terminal.SetMaxTotalModelBytesForTest(modelBytes * 3 / 4)
	defer restoreBytes()

	// Wait for both to reach their prompts first, and ensure they have fully
	// settled (no model growth for 250 ms). This guarantees the shell is
	// responsive before we write the sleep command, so waitNotIdle completes
	// quickly (< 1 s) rather than risking the engine's 10-second maintenance
	// ticker firing and causing a data race on maxTotalSessions with the next test.
	waitIdle(t, eng, sid1, 10*time.Second)
	waitForSettled(t, eng, sid1, 10*time.Second)
	waitIdle(t, eng, sid2, 10*time.Second)
	waitForSettled(t, eng, sid2, 10*time.Second)

	// Start a long-running foreground process in both.
	require.NoError(t, eng.Write(ctx, sid1, []byte("sleep 9999\n")))
	require.NoError(t, eng.Write(ctx, sid2, []byte("sleep 9999\n")))
	waitNotIdle(t, eng, sid1, 10*time.Second)
	waitNotIdle(t, eng, sid2, 10*time.Second)

	// sid1 is "older" — it should be force-suspended first.
	base := time.Now().Add(-10 * time.Minute)
	terminal.SetLastActiveForTest(eng, sid1, base)
	terminal.SetLastActiveForTest(eng, sid2, base.Add(time.Minute))

	// Run maintenance: 2 live models (512 KB) > 384 KB ceiling → global ceiling fires.
	// No idle candidates → last-resort force-suspend of sid1 (oldest).
	terminal.RunMaintenanceOnceForTest(eng, ctx)

	// The force-suspend runs synchronously inside the sweep (WriteBuf then saveMeta
	// "suspended" before it returns), so once "suspended" is recorded the .buf is
	// already on disk. Wait on that durable state deterministically.
	require.Eventually(t, func() bool {
		return store.hasSavedWithState(sid1, "suspended")
	}, 10*time.Second, 10*time.Millisecond,
		"oldest session must be force-suspended by global ceiling last resort")

	// sid1 must be suspended (placeholder) and sid2 must still be live.
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

	// Shutdown must return promptly.
	done := make(chan struct{})
	go func() {
		eng.Shutdown()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not return within 5 s")
	}

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

	// Create one session — it starts detached (live, no clients).
	sid, err := eng.Create(ctx, "ws-stats", dir, nil)
	require.NoError(t, err)

	_, det, _, modelB, deg2, pp2 := eng.Stats()
	assert.Equal(t, 1, det, "one detached session")
	assert.Greater(t, modelB, int64(0), "model bytes must be positive for a live session")
	assert.Zero(t, deg2, "a healthy live session reports not-degraded")
	assert.Zero(t, pp2, "a healthy live session reports zero parse panics")

	// Suspend it — must move to suspended.
	waitIdle(t, eng, sid, 10*time.Second)
	deadline := time.After(15 * time.Second)
	for {
		_ = eng.Suspend(ctx, sid)
		if store.hasSavedWithState(sid, "suspended") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("session did not suspend within 15 s")
		case <-time.After(200 * time.Millisecond):
		}
	}

	_, detAfter, susp, _, _, _ := eng.Stats()
	assert.Zero(t, detAfter, "no detached sessions after suspend")
	assert.Equal(t, 1, susp, "one suspended session")

	_ = eng.Kill(ctx, sid)
}
