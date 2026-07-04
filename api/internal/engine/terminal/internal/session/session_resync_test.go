//go:build !windows

package session

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drainFrames pulls everything currently queued on ch without blocking.
func drainFrames(ch <-chan OutputFrame) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// waitSnapshotFrame scans frames arriving on ch until a Snapshot frame shows up
// or the timeout elapses.
func waitSnapshotFrame(ch <-chan OutputFrame, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case f, ok := <-ch:
			if !ok {
				return false
			}
			if f.Snapshot {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

// TestAttach_FirstFrameIsSnapshot pins the attach redraw's Snapshot marking:
// the client applies it onto a reset buffer, so it must be distinguishable
// from incremental output.
func TestAttach_FirstFrameIsSnapshot(t *testing.T) {
	s, err := New("sid-snap-attach", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	select {
	case f := <-ch:
		assert.True(t, f.Snapshot, "the attach redraw frame must be marked Snapshot")
		assert.NotEmpty(t, f.Data)
	case <-time.After(3 * time.Second):
		t.Fatal("no attach frame received")
	}
}

// TestResync_IdleShell_NoOp pins the gate: at an idle prompt xterm's native
// reflow is correct, so Resync must not emit anything.
func TestResync_IdleShell_NoOp(t *testing.T) {
	s, err := New("sid-resync-idle", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	// Let the shell reach its prompt (mirrors TestIsIdle_unix's settle).
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
	}
	time.Sleep(150 * time.Millisecond)
	idle := false
	for i := 0; i < 20; i++ {
		if s.IsIdle() {
			idle = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !idle {
		t.Skip("shell never settled idle in this environment")
	}

	drainFrames(ch)
	assert.False(t, s.Resync(), "Resync at an idle shell must be a no-op")
	assert.False(t, waitSnapshotFrame(ch, 300*time.Millisecond),
		"no snapshot frame may be emitted for an idle shell")
}

// TestResync_ForegroundApp_EmitsSnapshot pins the busy path: with a foreground
// child running, Resync re-emits the serialized model to attached clients.
func TestResync_ForegroundApp_EmitsSnapshot(t *testing.T) {
	s, err := New("sid-resync-busy", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	require.NoError(t, s.Write([]byte("sleep 9999\n")))

	busy := false
	for i := 0; i < 60; i++ {
		if !s.IsIdle() {
			busy = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.True(t, busy, "sleep child never took the foreground")

	// The attach redraw is also Snapshot-marked; drain everything queued so the
	// snapshot observed below is unambiguously the Resync one.
	drainFrames(ch)

	assert.True(t, s.Resync(), "Resync with a foreground app must emit")
	assert.True(t, waitSnapshotFrame(ch, 3*time.Second),
		"attached client must receive the resync Snapshot frame")
}

// TestResync_Placeholder_NoOp covers the model==nil guard.
func TestResync_Placeholder_NoOp(t *testing.T) {
	ph := NewPlaceholder("sid-resync-ph", "/bin/sh", "", "", nil)
	assert.False(t, ph.Resync())
}
