package terminal

import (
	"context"
	"time"
)

// RunMaintenanceOnceForTest exposes runMaintenanceOnce for unit tests so they
// can drive the maintenance sweep directly without waiting for the 10-second ticker.
func RunMaintenanceOnceForTest(eng Engine, ctx context.Context) {
	eng.(*terminalEngine).runMaintenanceOnce(ctx)
}

// SetSoftLimitPerWorkspaceForTest overrides the per-workspace detached-session
// soft limit and returns a restore function. Call defer restore() in tests.
func SetSoftLimitPerWorkspaceForTest(n int) (restore func()) {
	old := softLimitPerWorkspace
	softLimitPerWorkspace = n
	return func() { softLimitPerWorkspace = old }
}

// SetMaxTotalSessionsForTest overrides the global session-count ceiling and
// returns a restore function.
func SetMaxTotalSessionsForTest(n int) (restore func()) {
	old := maxTotalSessions
	maxTotalSessions = n
	return func() { maxTotalSessions = old }
}

// SetMaxTotalModelBytesForTest overrides the global model-bytes ceiling and
// returns a restore function.
func SetMaxTotalModelBytesForTest(n int64) (restore func()) {
	old := maxTotalModelBytes
	maxTotalModelBytes = n
	return func() { maxTotalModelBytes = old }
}

// SetLastActiveForTest directly sets the last-active timestamp for a session,
// allowing tests to control ordering without real time delays.
func SetLastActiveForTest(eng Engine, id string, t time.Time) {
	e := eng.(*terminalEngine)
	e.mu.Lock()
	e.lastActive[id] = t
	e.mu.Unlock()
}

// IsIdleForTest reports whether the session with the given ID is currently idle.
// Exposed for tests that need to wait for a session's foreground process state
// without having access to the concrete *session.Session type.
func IsIdleForTest(eng Engine, id string) bool {
	e := eng.(*terminalEngine)
	s, ok := e.reg.Get(id)
	if !ok {
		return false
	}
	return s.IsIdle()
}

// SnapshotLenForTest returns the number of bytes in the session's current
// serialized screen-model snapshot. Tests use this to wait until the shell has emitted at least one byte
// (prompt output) before triggering a cadence-flush maintenance sweep.
func SnapshotLenForTest(eng Engine, id string) int {
	e := eng.(*terminalEngine)
	s, ok := e.reg.Get(id)
	if !ok {
		return 0
	}
	// Non-mutating: SerializedLen does NOT consume the dirty bit (unlike Snapshot), so
	// polling it as a readiness/settle signal never makes a later cadence flush skip.
	return s.SerializedLen()
}

// StopMaintenanceForTest stops the background maintenance ticker goroutine
// without killing any active sessions. Call this immediately after New() in any
// test that either drives maintenance manually via RunMaintenanceOnceForTest or
// mutates the package-level limit vars (SetSoftLimitPerWorkspaceForTest,
// SetMaxTotalSessionsForTest, SetMaxTotalModelBytesForTest). Stopping the ticker
// ensures no background goroutine reads the limit vars concurrently with the
// test's writes, eliminating the data race under -race.
//
// The engine's Shutdown() remains safe to call afterwards: stopOnce ensures
// close(te.stop) is idempotent.
func StopMaintenanceForTest(eng Engine) {
	te := eng.(*terminalEngine)
	te.stopOnce.Do(func() { close(te.stop) })
}

// HasSessionMuForTest reports whether a sessionMu entry exists for the given
// session id. Exposed so tests can assert that dropUnrestorable (and other
// cleanup paths) correctly prune the per-session lifecycle mutex map.
func HasSessionMuForTest(eng Engine, id string) bool {
	_, ok := eng.(*terminalEngine).sessionMu.Load(id)
	return ok
}
