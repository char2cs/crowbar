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

// TestModelDriven_BurstCoalescesFrames proves the Task 7 adaptive frame
// clock: a burst of chunks arriving faster than minEmitInterval must
// coalesce into exactly one immediate frame (the cold-clock chunk that stamps
// lastEmitAt) plus one trailing-timer frame for everything else in the
// window — never one frame per chunk.
//
// Built directly on newBareSession + pumpStep (the pattern the boundary
// tests above use) rather than a live PTY: TestModelDriven_BurstCoalescesFrames
// previously drove a real `seq 1 200` through a live shell and asserted
// frames < 100, which flaked under CPU contention — a descheduled pump
// goroutine can leave the immediate-emit branch (elapsed >= minEmitInterval)
// true for MULTIPLE chunks in the same logical burst, inflating the frame
// count independent of the coalescing logic actually under test. Driving
// pumpStep directly removes the scheduler from the timing-sensitive path
// entirely: every chunk in the burst loop below is fed well within one 8ms
// window by construction, so the assertion pins the coalescing property
// itself instead of hoping the host stays fast enough.
func TestModelDriven_BurstCoalescesFrames(t *testing.T) {
	s := newBareSession("sid-md-burst", "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 2000)
	s.model = m
	s.serializer = ser
	s.modelDriven = true
	s.emitter = model.NewDiffEmitter()

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	_, ok := waitFrame(t, ch, time.Second)
	require.True(t, ok, "attach must deliver an initial snapshot")

	// The clock starts cold (lastEmitAt is zero), so this first chunk emits
	// immediately and stamps lastEmitAt to "now" — the one immediate frame
	// the burst is allowed.
	s.pumpStep([]byte("line0\n"))
	_, ok = waitFrame(t, ch, time.Second)
	require.True(t, ok, "first chunk must emit immediately (cold clock)")

	// Feed the rest of the burst — 50 chunks — pinning s.lastEmitAt to "now"
	// immediately before each pumpStep call, and defusing every timer it
	// arms except the very last one. This is deliberate, not just a speed
	// hack: under -race, 50 back-to-back lock/unlock + model-write cycles
	// can themselves take longer in wall-clock time than the 8ms window
	// (observed 9-19ms locally). A naive "let each chunk's own real timer
	// race the rest of the loop" version is exactly as flaky as the old
	// live-PTY test it replaces — the trailing timer armed by chunk 1 can
	// fire mid-loop while chunks 2-50 are still landing, producing more than
	// one trailing frame for reasons that have nothing to do with the
	// coalescing logic under test.
	//
	// So each iteration: pin lastEmitAt to "now" (this chunk is always
	// inside the window, regardless of real elapsed time), pumpStep it
	// (writes the model + arms a fresh trailing timer, since the previous
	// one was just defused), then immediately stop that timer again — EXCEPT
	// on the last chunk, where the timer is left armed to fire for real.
	// Stopping a timer never discards the chunk's contribution: pumpStep
	// already wrote it into the model before scheduleEmitLocked runs, so an
	// undefused-but-uncommitted delta simply accumulates until the surviving
	// timer's real Emit picks up the full accumulated diff at the end. This
	// deterministically simulates "50 chunks landed inside one coalesce
	// window" without needing the test goroutine to outrun -race
	// instrumentation.
	const burstChunks = 50
	for i := 1; i <= burstChunks; i++ {
		s.mu.Lock()
		s.lastEmitAt = time.Now()
		s.mu.Unlock()
		s.pumpStep([]byte("x\n"))
		if i < burstChunks {
			s.mu.Lock()
			s.stopEmitTimerLocked()
			s.mu.Unlock()
		}
	}

	s.mu.Lock()
	armed := s.emitTimer != nil
	s.mu.Unlock()
	require.True(t, armed, "test setup: the final burst chunk must leave the trailing timer armed")

	// No frame may arrive synchronously for any of the 50 coalesced chunks —
	// only the trailing timer, once it fires, may deliver one.
	select {
	case extra := <-ch:
		t.Fatalf("unexpected synchronous frame during the coalesce window: %+v", extra)
	case <-time.After(minEmitInterval / 2):
	}

	// Poll-wait for the trailing timer's single flush.
	_, ok = waitFrame(t, ch, 3*minEmitInterval)
	require.True(t, ok, "trailing timer must flush the coalesced burst")

	// Exactly one frame — no more — must follow the trailing flush.
	select {
	case extra := <-ch:
		t.Fatalf("unexpected second trailing frame (coalescing failed): %+v", extra)
	case <-time.After(3 * minEmitInterval):
	}
}

// TestModelDriven_BurstOverLivePTYDeliversAllOutput is a live-PTY smoke test
// kept alongside the deterministic coalescing test above: it asserts only a
// contention-proof property (all 200 lines eventually arrive), never a frame
// COUNT, so it cannot flake under CPU pressure the way the old frames < 100
// assertion did while still exercising the real spawn → pump → pumpStep path
// end to end against a live shell.
func TestModelDriven_BurstOverLivePTYDeliversAllOutput(t *testing.T) {
	s, err := NewModelDriven("sid-md-burst-live", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 2000)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	// settle: drain attach snapshot + prompt
	_, _ = collect(ch, 500*time.Millisecond)

	require.NoError(t, s.Write([]byte("seq 1 200\n")))
	data, _ := collect(ch, 5*time.Second)
	assert.Contains(t, data, "200", "burst output must fully arrive regardless of coalescing frame count")
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

// TestModelDriven_TeardownStopsTrailingTimer pins the shutdown() guard
// (Task 7): if a trailing frame-clock timer is still armed when the session
// tears down, shutdown must stop it under the same s.mu hold that closes
// s.done, so the timer can never deliver a frame — or panic on a nil model/
// closed-channel send — after teardown. This is the deterministic
// counterpart to the <-s.done check inside the timer callback itself: it
// pins the guard against regression (e.g. someone reordering shutdown to
// close s.done before stopping the timer, or dropping the stop call
// entirely and relying solely on the done-channel check, which would still
// let the timer goroutine wake up and race the close of s.clients).
func TestModelDriven_TeardownStopsTrailingTimer(t *testing.T) {
	s := newBareSession("sid-md-teardown-timer", "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 200)
	s.model = m
	s.serializer = ser
	s.modelDriven = true
	s.emitter = model.NewDiffEmitter()

	// Attach a client BEFORE arming the timer so there is a channel to drain
	// and count frames on after teardown.
	ch, err := s.Attach()
	require.NoError(t, err)
	_, ok := waitFrame(t, ch, time.Second)
	require.True(t, ok, "attach must deliver an initial snapshot")

	// Cold-clock immediate emit, then a second chunk inside the window arms
	// the trailing timer — same two-step pattern as the Attach/Resync
	// disarm tests above.
	s.pumpStep([]byte("echo A\n"))
	_, ok = waitFrame(t, ch, time.Second)
	require.True(t, ok, "first chunk must emit immediately (cold clock)")

	s.pumpStep([]byte("echo B\n"))
	s.mu.Lock()
	armed := s.emitTimer != nil
	s.mu.Unlock()
	require.True(t, armed, "test setup: second chunk inside the window must arm the trailing timer")

	// Kill on a placeholder-shaped bare session (ptmx == nil) takes the
	// direct shutdown() branch, exercising the exact guard under test: a
	// still-armed timer at teardown time.
	require.NotPanics(t, func() { s.Kill() })

	select {
	case <-s.Done():
	case <-time.After(time.Second):
		t.Fatal("Kill must close s.done")
	}

	// Drain whatever shutdown enqueued (its own close(cl.send) fan-out, if
	// any) and wait well past the coalesce window the armed timer was due to
	// fire in — no frame may arrive from the stopped timer, and the channel
	// must end up closed with nothing further trickling in.
	frames := 0
	deadline := time.After(2 * minEmitInterval)
drain:
	for {
		select {
		case f, ok := <-ch:
			if !ok {
				break drain
			}
			frames++
			t.Logf("unexpected frame after teardown: %+v", f)
		case <-deadline:
			break drain
		}
	}
	assert.Equal(t, 0, frames, "no frame may arrive after teardown (trailing timer must have been stopped)")
}

// TestModelDriven_CPRQueryAnsweredToPTY proves the daemon answers a cursor-position-report
// query (ESC[6n) from the model under the model-driven flag (spec §3.8): the client xterm
// never sees the query in this mode, so without the session's response sink writing the
// model's synthesized reply back to the PTY master, an app reading the reply from stdin
// (here, the shell's `read`) would hang/time out.
func TestModelDriven_CPRQueryAnsweredToPTY(t *testing.T) {
	s, err := NewModelDriven("sid-md-cpr", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	_, _ = collect(ch, 500*time.Millisecond)

	// `read -s -t 3 REPLY` after emitting ESC[6n: the shell captures whatever
	// comes back on stdin. If the daemon answers, REPLY holds the CPR and the
	// marker prints; with no answer, read times out and prints NOANSWER.
	//
	// Sent as ONE PTY line (single trailing '\n'): the whole line is queued to
	// the shell atomically and fully parsed/executed before `read` blocks, so
	// there is no leftover queued text for the async CPR reply to race against
	// (a two-line script races: the second line can reach the tty input queue
	// ahead of, or interleaved with, the reply). The two markers are written
	// as separate back-to-back printf calls (nothing between them in the
	// EXECUTED output) but are never contiguous in the bytes actually TYPED
	// (shell syntax sits between them there), so the PTY's canonical-mode
	// local echo of the command line itself cannot satisfy the assertions
	// below — only the genuinely executed branch's printf output can.
	script := "printf '\\x1b[6n'; IFS= read -rs -t 3 -d R REPLY; " +
		"if [ -n \"$REPLY\" ]; then printf 'CPR'; printf -- '-ANSWERED\\n'; else printf 'NO'; printf 'ANSWER\\n'; fi\n"
	require.NoError(t, s.Write([]byte(script)))
	data, _ := collect(ch, 5*time.Second)
	assert.Contains(t, data, "CPR-ANSWERED")
	assert.NotContains(t, data, "NOANSWER")
}

// TestModelDriven_DegradedFlipUninstallsResponseSink proves the T8 review finding fix:
// once a model-driven session degrades to raw fallback (modelPanics > 0), the model's
// response sink must be uninstalled. Left armed, the model would keep answering device
// queries (e.g. CPR) from the PTY's raw bytes at the same time the client's own xterm —
// which now also sees those raw bytes — answers them too, so the app would receive
// DOUBLE replies to every query after the flip. One answerer at a time, always.
func TestModelDriven_DegradedFlipUninstallsResponseSink(t *testing.T) {
	s, err := NewModelDriven("sid-md-sink-flip", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	s.mu.Lock()
	installed := model.ResponseSinkInstalledForTest(s.model)
	s.mu.Unlock()
	require.True(t, installed, "test setup: a fresh model-driven session must spawn with the sink installed")

	forceModelPanicForTest(s)

	// Drive the flip through the useModelDrivenLocked latch (pumpStep -> writeModelLocked
	// -> useModelDrivenLocked), the same way TestModelDriven_DegradedFallsBackToRaw does.
	require.NoError(t, s.Write([]byte("echo SINK-FLIP-TRIGGER\n")))
	_, _ = collect(ch, 3*time.Second)

	s.mu.Lock()
	stillInstalled := model.ResponseSinkInstalledForTest(s.model)
	fellBack := s.modelDrivenFellBack
	s.mu.Unlock()
	require.True(t, fellBack, "test setup: the degraded flip must have latched")
	assert.False(t, stillInstalled, "the response sink must be uninstalled once the session flips to raw fallback")
}

// TestModelDriven_RawSessionNeverInstallsResponseSink is the raw-mode sibling of the
// degraded-flip test above: a plain (non-model-driven) session must never install a
// response sink at spawn — the client xterm is the sole answerer for raw sessions.
func TestModelDriven_RawSessionNeverInstallsResponseSink(t *testing.T) {
	s, err := New("sid-raw-no-sink", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	s.mu.Lock()
	installed := s.model != nil && model.ResponseSinkInstalledForTest(s.model)
	s.mu.Unlock()
	assert.False(t, installed, "a raw (non-model-driven) session must never install a response sink")
}

// TestModelDriven_CanaryStaysSilentOnHealthySession proves the dev
// divergence canary (Task 9, spec-brief) is a true no-op cost-wise unless
// enabled, and that a healthy model-driven session — where the shadow
// client-sim mirrors every emitted frame exactly the way a real client's
// terminal would — never reports a divergence.
func TestModelDriven_CanaryStaysSilentOnHealthySession(t *testing.T) {
	t.Setenv("CROWBAR_TERMINAL_MODEL_DRIVEN_CANARY", "1")
	s, err := NewModelDriven("sid-md-canary", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	require.NoError(t, s.Write([]byte("seq 1 50; echo CANARY-DONE\n")))
	data, _ := collect(ch, 3*time.Second)
	require.Contains(t, data, "CANARY-DONE")
	assert.Equal(t, int64(0), s.CanaryDivergences(), "healthy stream must never diverge")
}

// TestModelDriven_CanaryFiresOnInjectedDivergence proves the canary is
// falsifiable: without a way to deliberately desync the shadow sim from the
// authoritative model, TestModelDriven_CanaryStaysSilentOnHealthySession
// passing would be unfalsifiable — it could pass just as well with a canary
// that never actually compares anything. corruptCanarySimForTest writes
// bytes straight into the sim, outside mirrorCanaryLocked's normal mirroring,
// so the very next mirrored frame's grid-hash comparison must disagree.
func TestModelDriven_CanaryFiresOnInjectedDivergence(t *testing.T) {
	t.Setenv("CROWBAR_TERMINAL_MODEL_DRIVEN_CANARY", "1")
	s, err := NewModelDriven("sid-md-canary-neg", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	_, ok := waitFrame(t, ch, time.Second)
	require.True(t, ok, "attach must deliver an initial snapshot")

	corruptCanarySimForTest(s)

	require.NoError(t, s.Write([]byte("echo CANARY-FIRE\n")))
	data, _ := collect(ch, 3*time.Second)
	require.Contains(t, data, "CANARY-FIRE")
	assert.Greater(t, s.CanaryDivergences(), int64(0),
		"a deliberately corrupted canary sim must diverge from the authoritative model")
}

// TestModelDriven_CanaryDisabledByDefault proves the canary is opt-in: with
// the env var unset, s.canarySim stays nil even for a model-driven session,
// so CanaryDivergences() always reports 0 regardless of what the session
// does — the zero-cost path production runs.
func TestModelDriven_CanaryDisabledByDefault(t *testing.T) {
	s, err := NewModelDriven("sid-md-canary-off", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	require.NoError(t, s.Write([]byte("echo NO-CANARY\n")))
	data, _ := collect(ch, 3*time.Second)
	require.Contains(t, data, "NO-CANARY")
	assert.Equal(t, int64(0), s.CanaryDivergences())

	s.mu.Lock()
	nilSim := s.canarySim == nil
	s.mu.Unlock()
	assert.True(t, nilSim, "canary sim must stay nil when the env var is unset")
}

// TestModelDriven_CanaryPanicIsContained proves the canary is a true OBSERVER: a panic out of
// its shadow sim (mirrorCanaryLocked, Task 9) must never harm the session it is watching. Two
// failure modes exist without a recover boundary around mirrorCanaryLocked: (a) reached via
// scheduleEmitLocked's trailing time.AfterFunc goroutine, which has no safego.Recover above
// it — an unrecovered panic there crashes the whole daemon; (b) reached synchronously from
// pump(), where the outer recover would still tear down the real session via its own
// defer s.shutdown(). forceCanaryPanicForTest swaps in a sim whose Write always panics, so
// this test drives mirrorCanaryLocked straight into that boundary and asserts the session
// survives unharmed: it keeps streaming model-driven frames, modelPanics stays at 0 (a canary
// panic is an observer fault, not a model fault), CanaryDivergences is unaffected, and the
// canary sim is permanently disabled (nil) afterward.
func TestModelDriven_CanaryPanicIsContained(t *testing.T) {
	t.Setenv("CROWBAR_TERMINAL_MODEL_DRIVEN_CANARY", "1")
	s, err := NewModelDriven("sid-md-canary-panic", "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	_, ok := waitFrame(t, ch, time.Second)
	require.True(t, ok, "attach must deliver an initial snapshot")

	_, panicsBefore := s.Health()

	forceCanaryPanicForTest(s)

	// This write drives the panicking canary sim's Write via mirrorCanaryLocked. Without
	// the recover boundary, this either crashes the process (AfterFunc path) or tears the
	// session down via pump()'s defer s.shutdown() (synchronous path) — either way the
	// session would stop serving and this test would hang/fail on the requireOpen and
	// post-panic Write/collect below rather than passing cleanly.
	require.NoError(t, s.Write([]byte("echo CANARY-PANIC-SURVIVED\n")))
	data, _ := collect(ch, 3*time.Second)
	require.Contains(t, data, "CANARY-PANIC-SURVIVED",
		"session must keep streaming model-driven frames after a canary panic")

	select {
	case <-s.Done():
		t.Fatal("session done closed after a recovered canary panic — the observer harmed the observed session")
	default:
	}

	_, panicsAfter := s.Health()
	assert.Equal(t, panicsBefore, panicsAfter,
		"a canary panic is an observer fault, not a model fault — modelPanics must not bump")
	assert.Equal(t, int64(0), s.CanaryDivergences(),
		"a canary panic must not be misreported as a divergence")

	s.mu.Lock()
	nilSim := s.canarySim == nil
	s.mu.Unlock()
	assert.True(t, nilSim, "canary sim must be permanently disabled (nil) after a recovered panic")

	// The canary being disabled must not affect the session's core function: it keeps
	// serving model-driven output normally on a further write.
	require.NoError(t, s.Write([]byte("echo CANARY-STILL-SERVING\n")))
	data2, _ := collect(ch, 3*time.Second)
	require.Contains(t, data2, "CANARY-STILL-SERVING")
}
