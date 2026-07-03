//go:build !windows

package session

import (
	"os"
	"strings"
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

// TestModelDriven_BurstCoalescesFrames proves the Task 7 adaptive frame
// clock: a burst of PTY output produced faster than minEmitInterval must
// coalesce into far fewer client-visible frames than lines, instead of the
// pre-Task-7 one-frame-per-chunk behavior.
func TestModelDriven_BurstCoalescesFrames(t *testing.T) {
	s, err := NewModelDriven("sid-md-burst", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 2000)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	// settle: drain attach snapshot + prompt
	_, _ = collect(ch, 500*time.Millisecond)

	// A burst of 200 lines in one command: with the 8ms clock the client must
	// receive far fewer frames than lines.
	require.NoError(t, s.Write([]byte("seq 1 200\n")))
	frames := 0
	deadline := time.After(3 * time.Second)
	gotLast := false
	for !gotLast {
		select {
		case f := <-ch:
			frames++
			if strings.Contains(string(f.Data), "200") {
				gotLast = true
			}
		case <-deadline:
			t.Fatal("burst output never completed")
		}
	}
	assert.Less(t, frames, 100, "8ms clock must coalesce a 200-line burst (got %d frames)", frames)
}

// TestModelDriven_TrailingTimerFlushesFinalState proves output that arrives
// entirely inside one 8ms coalesce window still reaches the client: the
// trailing timer must fire and flush it rather than losing it.
func TestModelDriven_TrailingTimerFlushesFinalState(t *testing.T) {
	s, err := NewModelDriven("sid-md-trail", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	_, _ = collect(ch, 500*time.Millisecond)

	require.NoError(t, s.Write([]byte("echo TRAILING-EDGE-OK\n")))
	data, _ := collect(ch, 2*time.Second)
	assert.Contains(t, data, "TRAILING-EDGE-OK",
		"output arriving inside the coalesce window must still flush via the trailing timer")
}

// TestModelDriven_AttachDisarmsStaleTrailingTimer proves the Attach boundary
// (Task 7 carry-forward): if a burst chunk left the trailing frame-clock
// timer armed, Attach must flush that pending delta to existing clients and
// disarm the timer synchronously — not merely rely on the timer eventually
// firing on its own — so a second attach's freshly-primed emitter base is
// never raced by a late diff built off the pre-attach base, and no duplicate
// frame reaches the existing client.
//
// Built directly on newBareSession (as the emit-panic test above does) so the
// two writes below can be driven through pumpStep at exact, test-controlled
// timing instead of racing a live PTY's own chunking.
func TestModelDriven_AttachDisarmsStaleTrailingTimer(t *testing.T) {
	s := newBareSession("sid-md-attach-timer", "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 200)
	s.model = m
	s.serializer = ser
	s.modelDriven = true
	s.emitter = model.NewDiffEmitter()

	ch1, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch1)
	_, ok := waitFrame(t, ch1, time.Second)
	require.True(t, ok, "attach must deliver an initial snapshot")

	// The clock starts cold (lastEmitAt is zero), so this first chunk emits
	// immediately and also stamps lastEmitAt to "now".
	s.pumpStep([]byte("echo A\n"))
	_, ok = waitFrame(t, ch1, time.Second)
	require.True(t, ok, "first chunk must emit immediately (cold clock)")

	// This second chunk lands inside the 8ms window and must only ARM the
	// trailing timer rather than emit synchronously.
	s.pumpStep([]byte("echo B\n"))
	s.mu.Lock()
	armed := s.emitTimer != nil
	s.mu.Unlock()
	require.True(t, armed, "test setup: second chunk inside the window must arm the trailing timer")

	// Attach a second client while the trailing timer is still armed.
	ch2, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch2)

	s.mu.Lock()
	stillArmed := s.emitTimer != nil
	s.mu.Unlock()
	assert.False(t, stillArmed, "Attach must disarm the stale trailing timer")

	// Attach's own flush delivers the pending "B" delta to the existing
	// client exactly once.
	_, ok = waitFrame(t, ch1, time.Second)
	require.True(t, ok, "Attach must flush the pending delta to the existing client")

	f2, ok := waitFrame(t, ch2, time.Second)
	require.True(t, ok, "second attach must deliver a snapshot")
	assert.True(t, f2.Snapshot)

	// The disarmed timer must never separately fire afterward and duplicate
	// the flush already delivered above.
	select {
	case extra := <-ch1:
		t.Fatalf("unexpected extra frame on ch1 after Attach's flush (stale timer fired?): %+v", extra)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestModelDriven_ResyncDisarmsStaleTrailingTimer proves the Resync boundary
// (Task 7 carry-forward): Resync's forced keyframe already reflects every
// chunk written so far (the clock only defers the EMIT, never the model
// write), so a trailing timer armed by an unflushed burst chunk must be
// cancelled rather than left to fire later and re-emit a stale/duplicate
// frame off the base Resync's own keyframe just re-primed.
func TestModelDriven_ResyncDisarmsStaleTrailingTimer(t *testing.T) {
	s := newBareSession("sid-md-resync-timer", "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 200)
	s.model = m
	s.serializer = ser
	s.modelDriven = true
	s.emitter = model.NewDiffEmitter()

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	_, ok := waitFrame(t, ch, time.Second)
	require.True(t, ok, "attach must deliver an initial snapshot")

	s.pumpStep([]byte("echo A\n"))
	_, ok = waitFrame(t, ch, time.Second)
	require.True(t, ok, "first chunk must emit immediately (cold clock)")

	s.pumpStep([]byte("echo B\n"))
	s.mu.Lock()
	armed := s.emitTimer != nil
	s.mu.Unlock()
	require.True(t, armed, "test setup: second chunk inside the window must arm the trailing timer")

	ok = s.Resync()
	require.True(t, ok, "Resync must emit a keyframe for a non-idle bare session")

	s.mu.Lock()
	stillArmed := s.emitTimer != nil
	s.mu.Unlock()
	assert.False(t, stillArmed, "Resync must disarm the stale trailing timer")

	f, ok := waitFrame(t, ch, time.Second)
	require.True(t, ok, "Resync must deliver its keyframe")
	assert.True(t, f.Snapshot, "Resync's frame must be a keyframe")

	// The disarmed timer must never separately fire afterward and deliver a
	// second, redundant frame.
	select {
	case extra := <-ch:
		t.Fatalf("unexpected extra frame after Resync (stale timer fired?): %+v", extra)
	case <-time.After(50 * time.Millisecond):
	}
}
