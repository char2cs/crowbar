package terminal

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateCommand_RegistersSession(t *testing.T) {
	e := New()
	defer e.Shutdown()
	id, err := e.CreateCommand(context.Background(), "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "sleep 1"}, os.Environ(), nil)
	require.NoError(t, err)
	require.NotEmpty(t, id)
	require.True(t, e.SessionExists(context.Background(), id))
	require.Contains(t, e.ListSessionsForWorkspace("ws1"), id)
}

// TestCreateCommand_OnExitFiresOnceSessionEnds guards the resource-leak fix
// (agent.Usecase.spawnSegment's per-spawn tmp dir): the onExit callback passed
// to CreateCommand must fire exactly once reapOnDone has fully reaped a real,
// short-lived command's session — not before, and not more than once.
func TestCreateCommand_OnExitFiresOnceSessionEnds(t *testing.T) {
	e := New()
	defer e.Shutdown()
	ctx := context.Background()

	var fired int32
	id, err := e.CreateCommand(ctx, "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "true"}, os.Environ(), func() {
			atomic.AddInt32(&fired, 1)
		})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	require.Eventually(t, func() bool {
		return !e.SessionExists(ctx, id)
	}, 5*time.Second, 10*time.Millisecond, "session must be reaped after the command exits")

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&fired) == 1
	}, time.Second, 5*time.Millisecond, "onExit must fire exactly once after the session ends")

	// Give any (incorrect) duplicate invocation a chance to land, then confirm
	// the count is still exactly one.
	time.Sleep(50 * time.Millisecond)
	require.EqualValues(t, 1, atomic.LoadInt32(&fired), "onExit must not fire more than once")
}

// TestTerminateGraceful_OnExitFiresAfterGracefulSignal proves TerminateGraceful
// drives the exact same reap/cleanup chain as Kill (spec §8 must not regress
// the leak fix): the onExit callback passed to CreateCommand must still fire
// exactly once after a graceful SIGTERM lets the child exit on its own, well
// under the grace window.
func TestTerminateGraceful_OnExitFiresAfterGracefulSignal(t *testing.T) {
	e := New()
	defer e.Shutdown()
	ctx := context.Background()

	var fired int32
	id, err := e.CreateCommand(ctx, "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "sleep 30"}, os.Environ(), func() {
			atomic.AddInt32(&fired, 1)
		})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	start := time.Now()
	require.NoError(t, e.TerminateGraceful(ctx, id))
	elapsed := time.Since(start)

	require.Eventually(t, func() bool {
		return !e.SessionExists(ctx, id)
	}, 5*time.Second, 10*time.Millisecond, "session must be reaped after a graceful terminate")

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&fired) == 1
	}, time.Second, 5*time.Millisecond, "onExit must fire exactly once after a graceful terminate")

	assert.Less(t, elapsed, gracefulTerminateGrace, "a plain `sleep` dies on SIGTERM well before the grace window elapses")

	time.Sleep(50 * time.Millisecond)
	require.EqualValues(t, 1, atomic.LoadInt32(&fired), "onExit must not fire more than once")
}

// TestTerminateGraceful_FallsBackToKill_OnExitStillFires exercises the
// fallback-to-hard-kill branch through the full engine (not just the session
// package unit test): a child that ignores SIGTERM must still be reaped and
// its onExit fired exactly once, after consuming the (shortened, for test
// speed) grace window.
func TestTerminateGraceful_FallsBackToKill_OnExitStillFires(t *testing.T) {
	restore := SetGracefulTerminateGraceForTest(200 * time.Millisecond)
	defer restore()

	e := New()
	defer e.Shutdown()
	ctx := context.Background()

	var fired int32
	id, err := e.CreateCommand(ctx, "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "trap '' TERM; sleep 30"}, os.Environ(), func() {
			atomic.AddInt32(&fired, 1)
		})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	// Give the shell a moment to install the trap before terminating it, same
	// concern as the session-package unit test: a signal racing ahead of the
	// trap being installed would exit normally and defeat the point of this
	// test (proving the FALLBACK kill path still reaps cleanly).
	time.Sleep(100 * time.Millisecond)

	start := time.Now()
	require.NoError(t, e.TerminateGraceful(ctx, id))
	elapsed := time.Since(start)

	require.Eventually(t, func() bool {
		return !e.SessionExists(ctx, id)
	}, 5*time.Second, 10*time.Millisecond, "session must still be reaped when the child ignores SIGTERM")

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&fired) == 1
	}, time.Second, 5*time.Millisecond, "onExit must fire exactly once after the fallback kill")

	assert.GreaterOrEqual(t, elapsed, 200*time.Millisecond, "a signal-ignoring child must consume the full (shortened) grace window")
}

// TestEngine_TerminateGraceful_Unknown mirrors TestEngine_Kill_Unknown: an
// unknown session id must return a wrapped ErrSessionNotFound, not panic or
// silently succeed.
func TestEngine_TerminateGraceful_Unknown(t *testing.T) {
	e := New()
	defer e.Shutdown()
	err := e.TerminateGraceful(context.Background(), "nope")
	require.Error(t, err)
}

func TestWithTerminalDefaults_InjectsTERMWhenAbsent(t *testing.T) {
	// A command session started with an env lacking TERM ends up with the default.
	got := withTerminalDefaults([]string{"PATH=/usr/bin"})
	require.Contains(t, got, "TERM=xterm-256color")
	require.Contains(t, got, "COLORTERM=truecolor")
	// Does not override a caller-provided TERM.
	got = withTerminalDefaults([]string{"TERM=screen"})
	require.Contains(t, got, "TERM=screen")
	require.NotContains(t, got, "TERM=xterm-256color")
}
