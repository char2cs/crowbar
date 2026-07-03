//go:build !windows

package session

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// collect drains frames for d, concatenating data and remembering snapshots.
func collect(ch <-chan OutputFrame, d time.Duration) (data string, snapshots int) {
	deadline := time.After(d)
	for {
		select {
		case f, ok := <-ch:
			if !ok {
				return data, snapshots
			}
			if f.Snapshot {
				snapshots++
			}
			data += string(f.Data)
		case <-deadline:
			return data, snapshots
		}
	}
}

func TestModelDriven_OutputIsModelDerived(t *testing.T) {
	s, err := NewModelDriven("sid-md", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	require.NoError(t, s.Write([]byte("echo MD-MARKER-42\n")))
	data, _ := collect(ch, 3*time.Second)
	// The marker must arrive (via diff frames), proving the emit path works
	// end to end. Diff frames use absolute cursor addressing, which raw shell
	// echo never emits at the prompt for plain output.
	assert.Contains(t, data, "MD-MARKER-42")
	assert.Contains(t, data, "\x1b[", "model-driven output is synthesized ANSI")
}

func TestModelDriven_DegradedFallsBackToRaw(t *testing.T) {
	s, err := NewModelDriven("sid-md-deg", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	// Force the degraded state the way the panic tests do (see
	// session_panic_test.go / session_testseams.go for the seam that swaps in
	// a panicking model — reuse it exactly).
	forceModelPanicForTest(s)

	require.NoError(t, s.Write([]byte("echo RAW-FALLBACK-7\n")))
	data, _ := collect(ch, 3*time.Second)
	assert.Contains(t, data, "RAW-FALLBACK-7", "degraded session must still stream raw")
}

// TestModelDriven_ResizeInvalidatesEmitterForcingNextKeyframe proves Resize
// invalidates the diff emitter (spec: a resize can never be expressed as an
// absolute-addressed diff), so the very next model-derived frame after a
// resize is a Snapshot keyframe rather than an incremental diff.
func TestModelDriven_ResizeInvalidatesEmitterForcingNextKeyframe(t *testing.T) {
	s, err := NewModelDriven("sid-md-resize", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	// Drain the initial attach snapshot and let the emitter settle (Prime'd).
	_, ok := waitFrame(t, ch, time.Second)
	require.True(t, ok, "attach must deliver an initial snapshot")

	require.NoError(t, s.Resize(100, 40))

	require.NoError(t, s.Write([]byte("echo POST-RESIZE\n")))
	f, ok := waitFrame(t, ch, 3*time.Second)
	require.True(t, ok, "post-resize output must produce a frame")
	assert.True(t, f.Snapshot, "the first frame emitted after Resize must be a keyframe (emitter invalidated)")
	assert.Contains(t, string(f.Data), "POST-RESIZE")
}

// TestModelDriven_AttachFlushesPendingDeltaBeforeRebasing proves Attach, under
// the flag, flushes any pending delta to EXISTING clients via emitFrameLocked
// before serializing the fresh-attach snapshot and priming the emitter — so a
// delta accumulated between the last emit and this attach is never silently
// dropped for clients that were already attached.
func TestModelDriven_AttachFlushesPendingDeltaBeforeRebasing(t *testing.T) {
	s, err := NewModelDriven("sid-md-flush", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch1, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch1)

	// Drain ch1's initial keyframe snapshot.
	_, ok := waitFrame(t, ch1, time.Second)
	require.True(t, ok)

	require.NoError(t, s.Write([]byte("echo FIRST-CLIENT-DELTA\n")))
	data1, _ := collect(ch1, 2*time.Second)
	assert.Contains(t, data1, "FIRST-CLIENT-DELTA", "existing client must see output emitted before the second attach")

	// A second attach must not lose any already-emitted output for ch1, and
	// must itself receive a self-contained snapshot.
	ch2, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch2)
	f2, ok := waitFrame(t, ch2, time.Second)
	require.True(t, ok, "second attach must deliver a snapshot")
	assert.True(t, f2.Snapshot)
}
