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

func TestMaintenance_CadenceFlush(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	sid := newReadyShell(t, eng, "chat-flush", dir)

	savesBefore := countSavedForSession(store, sid)
	terminal.RunMaintenanceOnceForTest(eng, ctx)

	assert.True(t, bufExists(store.dir, sid), ".buf must be written on first cadence flush")
	savesAfter := countSavedForSession(store, sid)
	assert.Greater(t, savesAfter, savesBefore, "meta must be saved on first cadence flush")
	savesQuiet := countSavedForSession(store, sid)

	terminal.RunMaintenanceOnceForTest(eng, ctx)
	assert.Equal(t, savesQuiet, countSavedForSession(store, sid),
		"no new meta save when session has no new output (dirty=false)")

	runShell(t, eng, sid, "echo dirty-again")
	terminal.RunMaintenanceOnceForTest(eng, ctx)
	assert.Greater(t, countSavedForSession(store, sid), savesQuiet,
		"a session with new output since the last flush must be flushed again")

	require.NoError(t, eng.Kill(ctx, sid))
}

func TestMaintenance_SoftLimit(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	restore := terminal.SetSoftLimitPerChatForTest(eng, 2)
	defer restore()
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	var sids [4]string
	for i := range sids {
		sids[i] = newReadyShell(t, eng, "chat-softlimit", dir)
	}

	base := time.Now().Add(-10 * time.Minute)
	for i, sid := range sids {
		terminal.SetLastActiveForTest(eng, sid, base.Add(time.Duration(i)*time.Minute))
	}

	terminal.RunMaintenanceOnceForTest(eng, ctx)

	assert.True(t, store.hasSavedWithState(sids[0], "suspended"),
		"oldest session must be suspended by soft limit")
	assert.True(t, store.hasSavedWithState(sids[1], "suspended"),
		"second-oldest session must be suspended by soft limit")

	assert.True(t, eng.SessionExists(ctx, sids[2]), "third session must still exist")
	assert.True(t, eng.SessionExists(ctx, sids[3]), "fourth (newest) session must still exist")

	assert.NoError(t, eng.Write(ctx, sids[2], []byte("echo still-alive\n")),
		"third session must still be writable (live)")
	assert.NoError(t, eng.Write(ctx, sids[3], []byte("echo still-alive\n")),
		"fourth session must still be writable (live)")

	for _, sid := range sids {
		_ = eng.Kill(ctx, sid)
	}
}

func TestMaintenance_RunningNeverIdleSuspended(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	restore := terminal.SetSoftLimitPerChatForTest(eng, 1)
	defer restore()
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	sidIdle := newReadyShell(t, eng, "chat-running", dir)

	sidRunning := newReadyShell(t, eng, "chat-running", dir)
	startForeground(t, eng, sidRunning)

	base := time.Now().Add(-10 * time.Minute)
	terminal.SetLastActiveForTest(eng, sidRunning, base)
	terminal.SetLastActiveForTest(eng, sidIdle, base.Add(time.Minute))

	terminal.RunMaintenanceOnceForTest(eng, ctx)
	assert.True(t, store.hasSavedWithState(sidIdle, "suspended"),
		"the idle over-limit session must be suspended in a single sweep")

	assert.NoError(t, eng.Write(ctx, sidRunning, []byte("echo running\n")),
		"running session must NOT be suspended by the soft-limit (idle-gated) path")

	_ = eng.Kill(ctx, sidIdle)
	_ = eng.Kill(ctx, sidRunning)
}

func TestMaintenance_GlobalForceLastResort(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	sid1 := newReadyShell(t, eng, "chat-force", dir)
	sid2 := newReadyShell(t, eng, "chat-force", dir)

	_, _, _, modelBytes, _, _ := eng.Stats()
	restoreBytes := terminal.SetMaxTotalModelBytesForTest(eng, modelBytes*3/4)
	defer restoreBytes()

	startForeground(t, eng, sid1)
	startForeground(t, eng, sid2)

	base := time.Now().Add(-10 * time.Minute)
	terminal.SetLastActiveForTest(eng, sid1, base)
	terminal.SetLastActiveForTest(eng, sid2, base.Add(time.Minute))

	terminal.RunMaintenanceOnceForTest(eng, ctx)

	assert.True(t, store.hasSavedWithState(sid1, "suspended"),
		"oldest session must be force-suspended by global ceiling last resort")

	bufData, readErr := os.ReadFile(filepath.Join(store.dir, sid1+".buf"))
	require.NoError(t, readErr, ".buf must exist after force-suspend")
	assert.Contains(t, string(bufData), "suspended to free resources",
		"force-suspend buf must contain the resource-freed notice")

	assert.NoError(t, eng.Write(ctx, sid2, []byte("echo alive\n")),
		"sid2 must still be live after global force-suspend of sid1")

	_ = eng.Kill(ctx, sid1)
	_ = eng.Kill(ctx, sid2)
}

func TestMaintenance_CommandSessionsNeverSuspended(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	restoreSoft := terminal.SetSoftLimitPerChatForTest(eng, 0)
	defer restoreSoft()
	restoreCeiling := terminal.SetMaxTotalSessionsForTest(eng, 1)
	defer restoreCeiling()
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	sid1, err := eng.CreateCommand(ctx, "chat-cmd-ceiling", dir,
		[]string{"/bin/sh", "-c", "sleep 9999"}, os.Environ(), nil)
	require.NoError(t, err)
	sid2, err := eng.CreateCommand(ctx, "chat-cmd-ceiling", dir,
		[]string{"/bin/sh", "-c", "sleep 9999"}, os.Environ(), nil)
	require.NoError(t, err)

	base := time.Now().Add(-10 * time.Minute)
	terminal.SetLastActiveForTest(eng, sid1, base)
	terminal.SetLastActiveForTest(eng, sid2, base.Add(time.Minute))

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

func TestMaintenance_ShutdownStopsGoroutine(t *testing.T) {
	eng := terminal.New()
	ctx := context.Background()

	eng.Shutdown()

	assert.NotPanics(t, func() { eng.Shutdown() })

	assert.NotPanics(t, func() {
		terminal.RunMaintenanceOnceForTest(eng, ctx)
	})
}

func TestEngine_Stats(t *testing.T) {
	pinShell(t)

	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()
	store := newFakeMetaStore(t)
	eng.SetMetaStore(store)
	defer eng.Shutdown()

	a, d, s, rb, deg, pp := eng.Stats()
	assert.Zero(t, a+d+s, "no sessions at start")
	assert.Zero(t, rb, "no model bytes at start")
	assert.Zero(t, deg, "a clean engine reports zero degraded sessions (§9.4)")
	assert.Zero(t, pp, "a clean engine reports zero parse panics (§9.4)")

	sid := newReadyShell(t, eng, "chat-stats", dir)

	_, det, _, modelB, deg2, pp2 := eng.Stats()
	assert.Equal(t, 1, det, "one detached session")
	assert.Greater(t, modelB, int64(0), "model bytes must be positive for a live session")
	assert.Zero(t, deg2, "a healthy live session reports not-degraded")
	assert.Zero(t, pp2, "a healthy live session reports zero parse panics")

	require.NoError(t, eng.Suspend(ctx, sid))
	require.True(t, store.hasSavedWithState(sid, "suspended"),
		"Suspend persists the suspended meta before returning")

	_, detAfter, susp, _, _, _ := eng.Stats()
	assert.Zero(t, detAfter, "no detached sessions after suspend")
	assert.Equal(t, 1, susp, "one suspended session")

	_ = eng.Kill(ctx, sid)
}
