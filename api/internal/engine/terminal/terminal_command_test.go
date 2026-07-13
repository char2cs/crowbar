package terminal

import (
	"context"
	"os"
	"strings"
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
	exits := make(chan struct{}, 8) // buffered: a (buggy) duplicate fire must not block the reaper
	id, err := e.CreateCommand(ctx, "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "true"}, os.Environ(), func() {
			atomic.AddInt32(&fired, 1)
			exits <- struct{}{}
		})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	// onExit IS the signal. Block on it rather than polling for its effects: reapOnDone
	// removes the session from the registry BEFORE it invokes onExit, so once this returns
	// the deregistration is already a fact and can be asserted directly.
	<-exits
	require.False(t, e.SessionExists(ctx, id), "session must be reaped after the command exits")

	// "Exactly once" needs a moment at which no further fire is POSSIBLE — not a 50 ms window
	// in which one merely didn't happen to land. Shutdown joins every reaper before it
	// returns, so afterwards the count is final and closed. (The deferred Shutdown is a no-op
	// second call; stopOnce makes it idempotent.)
	e.Shutdown()
	require.EqualValues(t, 1, atomic.LoadInt32(&fired), "onExit must fire exactly once, never more")
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
	exits := make(chan struct{}, 8)
	id, err := e.CreateCommand(ctx, "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "sleep 30"}, os.Environ(), func() {
			atomic.AddInt32(&fired, 1)
			exits <- struct{}{}
		})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	start := time.Now()
	require.NoError(t, e.TerminateGraceful(ctx, id))
	elapsed := time.Since(start)

	// Block on onExit; the deregistration precedes it inside reapOnDone, so assert it directly.
	<-exits
	require.False(t, e.SessionExists(ctx, id), "session must be reaped after a graceful terminate")

	// TIMING-BY-SUBJECT: the grace window is the BEHAVIOUR UNDER TEST here, not a
	// synchronisation device. The assertion measures how long TerminateGraceful took and
	// requires it to be well short of the window — i.e. that a plain `sleep` died on the
	// SIGTERM rather than being hard-killed at the end of the grace. Nothing waits on this
	// duration; it is an observation about the code's behaviour.
	assert.Less(t, elapsed, gracefulTerminateGrace,
		"a plain `sleep` dies on SIGTERM well before the grace window elapses")

	// Shutdown joins every reaper, so afterwards no further fire is possible.
	e.Shutdown()
	require.EqualValues(t, 1, atomic.LoadInt32(&fired), "onExit must fire exactly once, never more")
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
	exits := make(chan struct{}, 8)
	// The child ANNOUNCES that its trap is installed, and does so from inside the shell,
	// AFTER the trap builtin has run. Waiting for that announcement is what makes this test
	// honest: the old `time.Sleep(100 * time.Millisecond)` was a guess about how fast a
	// fork+exec of /bin/sh is, and if the guess lost, the SIGTERM would race ahead of the
	// trap, the child would die normally, and the test would silently exercise the GRACEFUL
	// path while claiming to prove the FALLBACK one.
	id, err := e.CreateCommand(ctx, "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "trap '' TERM; printf '%s%s\\n' TRAP PED; sleep 30"},
		os.Environ(), func() {
			atomic.AddInt32(&fired, 1)
			exits <- struct{}{}
		})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	// Block until the announcement is on the session's screen. A command session is pumped
	// and modelled exactly like a shell session, so the same screen seam applies. The token is
	// printed from two printf arguments so it cannot be matched against the argv itself.
	WaitScreenForTest(t, e, id, func(s string) bool {
		return strings.Contains(s, "TRAPPED")
	}, "the child to install its SIGTERM trap")

	start := time.Now()
	require.NoError(t, e.TerminateGraceful(ctx, id))
	elapsed := time.Since(start)

	<-exits
	require.False(t, e.SessionExists(ctx, id),
		"session must still be reaped when the child ignores SIGTERM")

	// TIMING-BY-SUBJECT: the grace window IS the behaviour under test. This asserts the
	// signal-ignoring child consumed the full (deliberately shortened, via the
	// SetGracefulTerminateGraceForTest seam) window before the fallback hard-kill — an
	// observation about elapsed time, not a wait on one.
	assert.GreaterOrEqual(t, elapsed, 200*time.Millisecond,
		"a signal-ignoring child must consume the full (shortened) grace window")

	e.Shutdown()
	require.EqualValues(t, 1, atomic.LoadInt32(&fired), "onExit must fire exactly once, never more")
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
