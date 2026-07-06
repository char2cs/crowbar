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
	s, err := newTestSession(t, "sid-terminate-graceful", dir)
	require.NoError(t, err)

	start := time.Now()
	const grace = 2 * time.Second
	s.Terminate(grace)
	elapsed := time.Since(start)

	select {
	case <-s.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("session did not terminate after Terminate")
	}

	assert.Less(t, elapsed, grace, "a plain shell must exit on SIGTERM well before the grace window elapses")

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

	markerRe := regexp.MustCompile(`MARKER-[0-9]+`)
	var buf bytes.Buffer
	deadline := time.After(5 * time.Second)
	confirmed := false
	for !confirmed {
		select {
		case f, ok := <-ch:
			if !ok {
				t.Fatal("channel closed before trap confirmation")
			}
			buf.Write(f.Data)
			if markerRe.Match(buf.Bytes()) {
				confirmed = true
			}
		case <-deadline:
			t.Fatalf("timeout waiting for shell to confirm the SIGTERM trap was installed; output so far: %q", buf.String())
		}
	}

	const grace = 200 * time.Millisecond
	start := time.Now()
	s.Terminate(grace)
	elapsed := time.Since(start)

	select {
	case <-s.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("session did not terminate after Terminate's fallback kill")
	}

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

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Terminate must return immediately for a placeholder session, not wait out the grace window")
	}

	select {
	case <-s.Done():
	case <-time.After(1 * time.Second):
		t.Fatal("placeholder session did not shut down after Terminate")
	}
}
