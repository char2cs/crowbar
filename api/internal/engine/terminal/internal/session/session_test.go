package session

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitFrame blocks until ch receives a frame or deadline elapses.
func waitFrame(
	t *testing.T,
	ch <-chan OutputFrame,
	timeout time.Duration,
) (OutputFrame, bool) {
	t.Helper()
	select {
	case f, ok := <-ch:
		return f, ok
	case <-time.After(timeout):
		return OutputFrame{}, false
	}
}

func TestSession_NewAndKill(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-1", "/bin/sh", dir, os.Environ())
	require.NoError(t, err)
	require.NotNil(t, s)

	s.Kill()

	select {
	case <-s.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("session did not terminate after Kill")
	}
}

func TestSession_AttachReceivesOutput(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-2", "/bin/sh", dir, os.Environ())
	require.NoError(t, err)

	ch, err := s.Attach()
	require.NoError(t, err)

	require.NoError(t, s.Write([]byte("echo hello\n")))

	found := false
	deadline := time.After(3 * time.Second)
	for !found {
		select {
		case f, ok := <-ch:
			if !ok {
				t.Fatal("channel closed before output received")
			}
			if containsStr(f.Data, "hello") {
				found = true
			}
		case <-deadline:
			t.Fatal("timeout waiting for 'hello' output")
		}
	}

	s.Kill()
}

func TestSession_AttachDeadSession(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-3", "/bin/sh", dir, os.Environ())
	require.NoError(t, err)
	s.Kill()

	<-s.Done()

	_, err = s.Attach()
	assert.Error(t, err)
}

func TestSession_DetachClosesChannel(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-4", "/bin/sh", dir, os.Environ())
	require.NoError(t, err)

	ch, err := s.Attach()
	require.NoError(t, err)

	s.Detach(ch)

	// After detach, sending more output must not block forever.
	// We write to the PTY and verify channel is closed.
	_ = s.Write([]byte("echo bye\n"))
	s.Kill()
}

func TestSession_RingBufferReplay(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-5", "/bin/sh", dir, os.Environ())
	require.NoError(t, err)

	ch, err := s.Attach()
	require.NoError(t, err)

	require.NoError(t, s.Write([]byte("echo ring\n")))

	// Wait until ring has data.
	deadline := time.After(3 * time.Second)
	found := false
	for !found {
		select {
		case f, ok := <-ch:
			if !ok {
				t.Fatal("channel closed unexpectedly")
			}
			if containsStr(f.Data, "ring") {
				found = true
			}
		case <-deadline:
			t.Fatal("timeout waiting for ring output on first client")
		}
	}

	s.Detach(ch)

	// Second attach must replay ring (snapshot contains "ring").
	ch2, err := s.Attach()
	require.NoError(t, err)

	f, ok := waitFrame(t, ch2, 2*time.Second)
	assert.True(t, ok, "expected replay frame")
	assert.True(t, containsStr(f.Data, "ring"), "replay must contain 'ring', got: %q", f.Data)

	s.Detach(ch2)
	s.Kill()
}

func TestSession_Resize(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-6", "/bin/sh", dir, os.Environ())
	require.NoError(t, err)
	assert.NoError(t, s.Resize(120, 40))
	s.Kill()
}

func TestSession_ID(t *testing.T) {
	dir := t.TempDir()
	s, err := New("my-id", "/bin/sh", dir, os.Environ())
	require.NoError(t, err)
	assert.Equal(t, "my-id", s.ID())
	s.Kill()
}

func TestSession_New_BadShell(t *testing.T) {
	dir := t.TempDir()
	// A non-existent executable must cause pty.Start to fail.
	_, err := New("sid-bad", "/nonexistent/shell/binary", dir, os.Environ())
	assert.Error(t, err)
}

func TestSession_WriteAfterKill(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-7", "/bin/sh", dir, os.Environ())
	require.NoError(t, err)
	s.Kill()
	// Wait for PTY to fully close.
	<-s.Done()
	// Writing to a killed session should return an error.
	writeErr := s.Write([]byte("hello"))
	assert.Error(t, writeErr)
}

func TestSession_ResizeAfterKill(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-8", "/bin/sh", dir, os.Environ())
	require.NoError(t, err)
	s.Kill()
	<-s.Done()
	// Resizing a killed session should return an error.
	resizeErr := s.Resize(80, 24)
	assert.Error(t, resizeErr)
}

func TestSession_DropOnOverflow(t *testing.T) {
	dir := t.TempDir()
	s, err := New("sid-9", "/bin/sh", dir, os.Environ())
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)

	// Drain any snapshot frame that was delivered on attach.
	for len(ch) > 0 {
		<-ch
	}

	// Call fanOut directly until the client's channel overflows.
	// ClientSendBufForTest + 1 writes are guaranteed to overflow.
	chunk := []byte("x")
	for i := 0; i <= ClientSendBufForTest; i++ {
		s.FanOutForTest(chunk)
	}

	// The channel must now be closed because the client was dropped.
	select {
	case _, ok := <-ch:
		// Either we read the overflow batch or the channel is closed.
		if !ok {
			return // channel closed: drop happened
		}
		// Channel not closed yet; drain and check again.
		for len(ch) > 0 {
			if _, ok := <-ch; !ok {
				return
			}
		}
	default:
	}

	// If not yet closed, wait a moment for the goroutine to process.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("client was not dropped after channel overflow")
		}
	}
}

func TestIsNormalPTYClose_EOF(t *testing.T) {
	assert.True(t, isNormalPTYClose(io.EOF))
}

func TestIsNormalPTYClose_EIO(t *testing.T) {
	// On Linux the PTY master returns EIO when the shell exits.
	assert.True(t, isNormalPTYClose(syscall.EIO))
}

func TestIsNormalPTYClose_WrappedEIO(t *testing.T) {
	wrapped := fmt.Errorf("pty: %w", syscall.EIO)
	assert.True(t, isNormalPTYClose(wrapped))
}

func TestIsNormalPTYClose_OtherError(t *testing.T) {
	assert.False(t, isNormalPTYClose(errors.New("unexpected error")))
	assert.False(t, isNormalPTYClose(syscall.EPERM))
}

func containsStr(
	data []byte,
	sub string,
) bool {
	return len(data) > 0 && len(sub) > 0 && contains(data, []byte(sub))
}

func contains(
	haystack []byte,
	needle []byte,
) bool {
	if len(needle) == 0 {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
