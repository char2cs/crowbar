//go:build !windows

package session

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/model"
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

// TestModelDriven_EmitPanicOnFlipDoesNotDropTheTriggeringChunk proves the boundary the
// review finding called out: writeModelLocked can succeed (the model consumed the
// chunk) while the EMIT path (emitLocked / the keyframe serializeLocked) panics and
// recovers, bumping modelPanics and flipping the session to raw for the NEXT chunk. If
// pumpStep did nothing else, that chunk's visual delta would be silently dropped —
// no frame goes out for it, and the flip only affects chunks after this one. pumpStep
// must detect the flip and fan the triggering chunk's raw bytes out instead, and the
// session must keep flowing normally (raw) afterward.
//
// It builds the Session directly (newBareSession + a real model/emitter) instead of
// spawning a live PTY: a spawned session's background pump goroutine reads the real
// shell asynchronously (startup mode-set sequences, prompt redraw) and would race this
// test's own direct pumpStep calls on the same synthetic chunks, nondeterministically
// landing the armed panic on the wrong chunk and masking a real drop. With no PTY there
// is nothing to race — pumpStep only ever runs on the exact chunks this test injects.
func TestModelDriven_EmitPanicOnFlipDoesNotDropTheTriggeringChunk(t *testing.T) {
	s := newBareSession("sid-md-emitpanic", "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 200)
	s.model = m
	s.serializer = ser
	s.modelDriven = true
	s.emitter = model.NewDiffEmitter()

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	// Drain the initial attach snapshot so it can't be mistaken for the fallback frame.
	_, ok := waitFrame(t, ch, time.Second)
	require.True(t, ok, "attach must deliver an initial snapshot")

	// Arm the emit-path panic for exactly the next emitLocked call, then drive that
	// exact chunk through pumpStep directly.
	forceEmitPanicForTest(s)
	s.pumpStep([]byte("LOST-CHUNK-GUARD"))

	f, ok := waitFrame(t, ch, time.Second)
	require.True(t, ok, "the chunk that triggered the emit-path panic must still reach the client via raw fallback, not be dropped")
	assert.Equal(t, "LOST-CHUNK-GUARD", string(f.Data),
		"the fallback frame must carry the triggering chunk's raw bytes verbatim")
	assert.False(t, f.Snapshot, "the fallback frame is a raw chunk, not a model snapshot")

	// The session must now be flipped to raw (modelPanics > 0) and keep streaming
	// normally through the pre-existing raw branch for every subsequent chunk.
	s.pumpStep([]byte("AFTER-FLIP"))
	f2, ok := waitFrame(t, ch, time.Second)
	require.True(t, ok, "session must keep streaming raw after the flip")
	assert.Equal(t, "AFTER-FLIP", string(f2.Data))
}
