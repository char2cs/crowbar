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

// PumpNotifyForTest returns the session's pump-progress signal (see
// session.PumpNotifyForTest): a coalescing edge published every time the pump has FULLY
// processed a chunk of PTY output. Blocking on it is how a test waits for a real shell to
// speak without guessing a duration. Returns nil if the session is unknown — a caller
// blocking on a nil channel blocks forever, which `go test -timeout` reports as the hang
// it is, rather than the silent pass a zero-value would give.
func PumpNotifyForTest(eng Engine, id string) <-chan struct{} {
	s, ok := eng.(*terminalEngine).reg.Get(id)
	if !ok {
		return nil
	}
	return s.PumpNotifyForTest()
}

// SerializedForTest returns the session's current serialized screen (non-consuming, so it
// never eats a dirty bit a later cadence flush is owed). Tests strip the ANSI and match on
// the resulting screen text to decide "the shell has finished doing what I asked".
func SerializedForTest(eng Engine, id string) []byte {
	s, ok := eng.(*terminalEngine).reg.Get(id)
	if !ok {
		return nil
	}
	return s.SerializedForTest()
}

// SessionDoneForTest returns the session's death channel. Waiters select on it alongside
// PumpNotifyForTest purely for DIAGNOSTICS: it is a real signal (the PTY closed), not a
// clock, and it turns "the shell exited and will never produce the output you're waiting
// for" from a silent 10-minute hang into an immediate, precise failure.
func SessionDoneForTest(eng Engine, id string) <-chan struct{} {
	s, ok := eng.(*terminalEngine).reg.Get(id)
	if !ok {
		return nil
	}
	return s.Done()
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

// SetGracefulTerminateGraceForTest overrides the package-level grace window
// TerminateGraceful waits before falling back to a hard kill, and returns a
// restore function. Lets a unit test exercise the fallback-to-hard-kill path
// (a signal-ignoring child) without a multi-second sleep.
func SetGracefulTerminateGraceForTest(d time.Duration) (restore func()) {
	old := gracefulTerminateGrace
	gracefulTerminateGrace = d
	return func() { gracefulTerminateGrace = old }
}

// BeginDrainForTest closes the engine's session-birth door WITHOUT killing or
// deregistering anything: it puts the engine in the state Shutdown occupies from the
// moment it starts draining until its kill loop reaches a given session.
//
// That window is where the restore refusal actually has to be right, and it is
// otherwise only reachable as a race. Once Shutdown has RETURNED, every placeholder
// has been deregistered, so a late Attach fails with ErrSessionNotFound and never
// reaches the restore path at all — a test that attached after Shutdown would prove
// nothing about it. Exposed so the refusal can be driven deterministically instead.
func BeginDrainForTest(eng Engine) {
	_ = eng.(*terminalEngine).reaps.drain()
}
