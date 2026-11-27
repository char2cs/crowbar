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

func TestSession_Terminate_GracefulExit_UsesSIGTERM(t *testing.T) {
	dir := t.TempDir()

	s, err := New(t.Context(), "sid-terminate-graceful", "/bin/cat", dir, "", testEnv(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	const grace = 2 * time.Second
	s.Terminate(grace)

	<-s.Done()

	require.NotNil(t, s.cmd.ProcessState, "graceful exit must still reap the child via cmd.Wait()")
	ws, ok := s.cmd.ProcessState.Sys().(syscall.WaitStatus)
	require.True(t, ok, "ProcessState.Sys() must be a syscall.WaitStatus on this platform")
	require.True(t, ws.Signaled(), "child must have died from a signal, not exited on its own")
	assert.Equal(t, syscall.SIGTERM, ws.Signal(), "child must have died from Terminate's SIGTERM, not a fallback SIGKILL")
}

func TestSession_Terminate_FallsBackToKill_WhenSignalIgnored(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "sid-terminate-fallback", dir)
	require.NoError(t, err)

	ch, err := s.Attach()
	require.NoError(t, err)

	require.NoError(t, s.Write([]byte("trap '' TERM; echo MARKER-$$\n")))

	markerRe := regexp.MustCompile(`MARKER-[0-9]+`)
	var buf bytes.Buffer
	for !markerRe.Match(buf.Bytes()) {
		f, ok := <-ch
		if !ok {
			t.Fatalf("channel closed before trap confirmation; output so far: %q", buf.String())
		}
		buf.Write(f.Data)
	}

	const grace = 200 * time.Millisecond
	start := time.Now()
	s.Terminate(grace)
	elapsed := time.Since(start)

	<-s.Done()

	assert.GreaterOrEqual(t, elapsed, grace, "a signal-ignoring child must consume the full grace window before the fallback kill")

	require.NotNil(t, s.cmd.ProcessState)
	ws, ok := s.cmd.ProcessState.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	require.True(t, ws.Signaled())
	assert.Equal(t, syscall.SIGKILL, ws.Signal(), "a SIGTERM-ignoring child must ultimately die from the fallback SIGKILL")
}

func TestSession_Terminate_NoProcessActsLikeKill(t *testing.T) {
	s := newBareSession("sid-terminate-placeholder", "/bin/sh", t.TempDir(), "")

	done := make(chan struct{})
	go func() {
		s.Terminate(5 * time.Second)
		close(done)
	}()

	<-done

	<-s.Done()
}
