//go:build !windows

package session

import (
	"testing"

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

// waitSnapshotFrame blocks until a Snapshot frame arrives on ch, skipping any incremental
// diff frames on the way. A closed channel (the session died) is the only way it reports
// false — there is no timeout: a snapshot that never arrives is a hang, and `go test -timeout`
// reports it with the blocked stack rather than a bespoke "not within 3s" message.
func waitSnapshotFrame(ch <-chan OutputFrame) bool {
	for f := range ch {
		if f.Snapshot {
			return true
		}
	}
	return false
}

// TestAttach_FirstFrameIsSnapshot pins the attach redraw's Snapshot marking:
// the client applies it onto a reset buffer, so it must be distinguishable
// from incremental output.
func TestAttach_FirstFrameIsSnapshot(t *testing.T) {
	s, err := New("sid-snap-attach", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	f, ok := waitFrame(t, ch)
	require.True(t, ok, "attach must deliver a redraw frame")
	assert.True(t, f.Snapshot, "the attach redraw frame must be marked Snapshot")
	assert.NotEmpty(t, f.Data)
}

// TestResync_IdleShell_NoOp pins the gate: at an idle prompt xterm's native
// reflow is correct, so Resync must not emit anything.
//
// The "nothing was emitted" half needs no observation window. Attaching only AFTER the shell
// has reached its prompt makes the client channel quiescent BY CONSTRUCTION: the shell is
// blocked on read(2) so no PTY output exists to fan out, and Attach itself flushes any pending
// delta and disarms the trailing frame-clock timer, so no deferred emit is in flight either.
// Resync's own emit path is synchronous (emitFrameLocked runs inside the call, under s.mu,
// enqueueing into the buffered client channel), so a keyframe — had one been produced — would
// already be queued the instant Resync returned. The old 300ms "no snapshot showed up" wait
// proved strictly less than the length check below, and could only have been wrong.
func TestResync_IdleShell_NoOp(t *testing.T) {
	s, err := New("sid-resync-idle", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	waitIdlePrompt(t, s) // idle is the PRECONDITION for the no-op; the prompt establishes it

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	drainFrames(ch) // the attach redraw is Snapshot-marked too; it is not the one under test

	assert.False(t, s.Resync(), "Resync at an idle shell must be a no-op")
	assert.Zero(t, len(ch), "no frame may be emitted for an idle shell")
}

// TestResync_ForegroundApp_EmitsSnapshot pins the busy path: with a foreground
// child running, Resync re-emits the serialized model to attached clients.
func TestResync_ForegroundApp_EmitsSnapshot(t *testing.T) {
	s, err := New("sid-resync-busy", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	waitPrompt(t, s)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	// startForeground blocks until the child announces itself — which, thanks to job control
	// (the job owns the terminal's foreground process group BEFORE it runs, and exec preserves
	// that pgroup), PROVES the foreground group has already flipped away from the shell. It
	// asserts !IsIdle() there with zero delay instead of polling for the flip.
	startForeground(t, s)

	// The attach redraw is also Snapshot-marked; drain everything queued so the
	// snapshot observed below is unambiguously the Resync one.
	drainFrames(ch)

	assert.True(t, s.Resync(), "Resync with a foreground app must emit")
	assert.True(t, waitSnapshotFrame(ch),
		"attached client must receive the resync Snapshot frame")
}

// TestResync_Placeholder_NoOp covers the model==nil guard.
func TestResync_Placeholder_NoOp(t *testing.T) {
	ph := NewPlaceholder("sid-resync-ph", "/bin/sh", "", "", nil)
	assert.False(t, ph.Resync())
}
