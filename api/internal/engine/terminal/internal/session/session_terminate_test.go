package session

import (
	"bytes"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSession_Terminate_GracefulExit_UsesSIGTERM proves the fast path (spec
// §8): a child that has no special SIGTERM handling exits on its own well
// within the grace window, and the recorded exit signal is SIGTERM — not
// Kill's SIGKILL — confirming Terminate actually sent a clean-exit signal
// rather than silently falling back to a hard kill.
func TestSession_Terminate_GracefulExit_UsesSIGTERM(t *testing.T) {
	dir := t.TempDir()

	// The child is `cat`, not a shell — and that is the whole point.
	//
	// An INTERACTIVE shell IGNORES SIGTERM (bash sets SIG_IGN for it precisely so that a
	// `kill 0` cannot take down your login shell). Verified on this platform: a fully
	// initialised /bin/sh on a PTY does not die on SIGTERM at all. So this test, when it
	// spawned a shell, could only ever pass by RACING that shell's startup — landing the
	// signal in the window before bash installs its handler. It won that race most of the
	// time and lost it under a loaded parallel -race run, whereupon the child survived the
	// grace window, took the fallback SIGKILL, and the SIGTERM assertion failed. The test was
	// not flaky; its premise was.
	//
	// `cat` holds the PTY open exactly as a shell does but keeps the DEFAULT SIGTERM
	// disposition, so "a child that honours SIGTERM exits, and Terminate never reaches its
	// fallback" becomes a property of the child rather than a bet on scheduling.
	s, err := New("sid-terminate-graceful", "/bin/cat", dir, "", testEnv(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	// grace is an INPUT to the code under test, not a wait this test performs: Terminate
	// returns as soon as the child exits, and only falls back to SIGKILL if it has not.
	const grace = 2 * time.Second
	s.Terminate(grace)

	// Terminate's shutdown closes Done; block on that real signal.
	<-s.Done()

	// The old `elapsed < grace` assertion is gone: it re-derived, from the clock, the very
	// thing the exit STATUS states outright. A child that survived the grace window would have
	// been SIGKILLed, so asserting the recorded signal is SIGTERM (below) is the same claim
	// made from evidence instead of from a stopwatch — and it cannot be wrong on a slow machine.
	require.NotNil(t, s.cmd.ProcessState, "graceful exit must still reap the child via cmd.Wait()")
	ws, ok := s.cmd.ProcessState.Sys().(syscall.WaitStatus)
	require.True(t, ok, "ProcessState.Sys() must be a syscall.WaitStatus on this platform")
	require.True(t, ws.Signaled(), "child must have died from a signal, not exited on its own")
	assert.Equal(t, syscall.SIGTERM, ws.Signal(), "child must have died from Terminate's SIGTERM, not a fallback SIGKILL")
}

// TestSession_Terminate_FallsBackToKill_WhenSignalIgnored proves the fallback
// path: a child that installs an empty SIGTERM trap (ignoring the signal
// entirely) survives the grace window, so Terminate must fall back to Kill's
// unconditional SIGKILL — confirmed both by the elapsed time (>= grace) and
// the recorded exit signal.
func TestSession_Terminate_FallsBackToKill_WhenSignalIgnored(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-terminate-fallback", dir)
	require.NoError(t, err)

	ch, err := s.Attach()
	require.NoError(t, err)

	// $$ expands to the shell's own PID only once bash actually EXECUTES the
	// echo — the PTY's cooked-mode local echo of our typed input reflects the
	// literal "MARKER-$$" text back immediately, before execution, so a plain
	// substring match on "MARKER-" would false-positive on that echo (racing
	// ahead of the trap actually being installed and intermittently sending
	// SIGTERM to a not-yet-trapped shell — precisely how this test flaked
	// under full-suite load). Requiring digits after "MARKER-" only matches
	// the real, POST-EXECUTION output.
	require.NoError(t, s.Write([]byte("trap '' TERM; echo MARKER-$$\n")))

	// Block on the shell's own confirmation that the trap is installed: the digits of $$ can
	// only be printed by the EXECUTED echo, which runs after the trap. No deadline — a
	// confirmation that never arrives is a hang, which `go test -timeout` reports.
	markerRe := regexp.MustCompile(`MARKER-[0-9]+`)
	var buf bytes.Buffer
	for !markerRe.Match(buf.Bytes()) {
		f, ok := <-ch
		if !ok {
			t.Fatalf("channel closed before trap confirmation; output so far: %q", buf.String())
		}
		buf.Write(f.Data)
	}

	// TIMING-BY-SUBJECT: the grace window IS the behaviour under test. Terminate's contract is
	// "wait up to grace for a clean exit, then hard-kill", so the elapsed-vs-grace relation is
	// the assertion, not a synchronisation guess. It is a LOWER bound (>= grace), which a slow
	// machine can only make more true — it fails only if Terminate returns EARLY, which is the
	// real bug it guards. The upper bound is left to the exit status: only the fallback path
	// can produce SIGKILL.
	const grace = 200 * time.Millisecond
	start := time.Now()
	s.Terminate(grace)
	elapsed := time.Since(start)

	// Terminate's fallback Kill runs shutdown, which closes Done; block on that real signal.
	<-s.Done()

	assert.GreaterOrEqual(t, elapsed, grace, "a signal-ignoring child must consume the full grace window before the fallback kill")

	require.NotNil(t, s.cmd.ProcessState)
	ws, ok := s.cmd.ProcessState.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	require.True(t, ws.Signaled())
	assert.Equal(t, syscall.SIGKILL, ws.Signal(), "a SIGTERM-ignoring child must ultimately die from the fallback SIGKILL")
}

// TestSession_Terminate_PlaceholderActsLikeKill mirrors Kill's placeholder
// behavior: a session with no live PTY has no process to signal, so Terminate
// must run shutdown() directly instead of hanging around for a grace window
// that can never elapse anything.
func TestSession_Terminate_PlaceholderActsLikeKill(t *testing.T) {
	s := NewPlaceholder("sid-terminate-placeholder", "/bin/sh", t.TempDir(), "", []byte("CRWB1"))

	done := make(chan struct{})
	go func() {
		s.Terminate(5 * time.Second)
		close(done)
	}()

	// Block on the real signal. A hand-rolled deadline here would only be a second,
	// weaker definition of "too slow"; if this never fires it is a hang, and `go test
	// -timeout` reports it with the blocked stack.
	<-done

	// Same for the shutdown itself: Done() closing IS "the placeholder shut down".
	<-s.Done()
}
