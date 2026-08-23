//go:build !windows

package session

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

// ---------------------------------------------------------------------------
// Frame-clock control.
//
// scheduleEmitLocked decides "emit this chunk now, or fold it into the pending trailing
// frame?" by comparing the WALL CLOCK against an 8ms window. Any test that needs a chunk to
// land INSIDE that window therefore has to guarantee its own setup runs in under 8ms — and it
// cannot: under -race, and especially under a full parallel test run, a couple of pumpSteps
// routinely take longer than that. Tests written that way do not flake, they measure their own
// runtime; they were failing "test setup: second chunk inside the window must arm the trailing
// timer" a few percent of the time.
//
// Freezing the session's clock makes "inside the window" true by construction, on any machine
// at any load. These helpers are the only supported way to do it.
// ---------------------------------------------------------------------------

// pinFrameClock freezes this session's frame clock and returns the instant it is pinned to.
func pinFrameClock(s *Session) time.Time {
	pinned := time.Now()
	s.SetNowForTest(func() time.Time { return pinned })
	return pinned
}

// parkTrailingTimer makes every subsequent chunk coalesce onto a trailing timer that CANNOT
// FIRE during the test: lastEmitAt is placed in the future, so now-lastEmitAt never reaches
// minEmitInterval (the chunk always coalesces) and the armed timer's delay comes out around an
// hour. Without this the timer is armed with a real 8ms delay and can fire mid-test — stealing
// the very flush the test is about to attribute to Attach/Resync/teardown.
func parkTrailingTimer(s *Session, pinned time.Time) {
	s.mu.Lock()
	s.lastEmitAt = pinned.Add(time.Hour)
	s.mu.Unlock()
}

// coldFrameClock forces the next chunk down the IMMEDIATE-emit branch (the elapsed test passes
// against the zero time), so a test can inject a synchronously-emitted barrier frame.
func coldFrameClock(s *Session) {
	s.mu.Lock()
	s.lastEmitAt = time.Time{}
	s.mu.Unlock()
}

// collectUntil blocks on the client's frames, concatenating their data, until one of the wants
// appears in the accumulated output; it returns everything seen up to and including that frame.
//
// It replaces the old `collect(ch, d)`, which drained for a fixed duration and then asserted on
// whatever had turned up. That is a bet that the shell is faster than d — and when the bet
// loses, the failure ("marker missing") points at the code under test rather than at the wager.
// Blocking on the marker itself makes the wait self-timing: a slow machine makes this slower,
// never wrong. A closed channel is a real signal (the session died); nothing more can arrive.
func collectUntil(
	t *testing.T,
	ch <-chan OutputFrame,
	wants ...string,
) string {
	t.Helper()
	var data string
	for {
		f, ok := <-ch
		if !ok {
			t.Fatalf("client channel closed before any of %q arrived; saw: %q", wants, data)
		}
		data += string(f.Data)
		for _, w := range wants {
			if strings.Contains(data, w) {
				return data
			}
		}
	}
}

// quiesce blocks until the shell is at its prompt and drains everything that produced —
// the attach snapshot and the prompt's own frames — so a later assertion sees only the frames
// its own action caused. It is the clock-free replacement for `collect(ch, 500ms)` as a "let it
// settle" step: the prompt is the shell stating it has finished, not a duration hoping it has.
func quiesce(t *testing.T, s *Session, ch <-chan OutputFrame) {
	t.Helper()
	waitPrompt(t, s)
	drainFrames(ch)
}

func TestModelDriven_OutputIsModelDerived(t *testing.T) {
	s, err := New("sid-md", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	require.NoError(t, s.Write([]byte("echo MD-MARKER-42\n")))
	// The marker must arrive (via diff frames), proving the emit path works
	// end to end. Diff frames use absolute cursor addressing, which raw shell
	// echo never emits at the prompt for plain output.
	data := collectUntil(t, ch, "MD-MARKER-42")
	assert.Contains(t, data, "\x1b[", "model-driven output is synthesized ANSI")
}

// TestModelDriven_HealthyAttachSnapshotHasNoPendingInput pins the I3 fix: a
// healthy (model-driven) attach snapshot must end EXACTLY at the serializer
// output, with no buffered mid-sequence partial appended. Under model-driven
// emission the client only ever receives model-derived frames, so the raw
// continuation of a partial escape never arrives — appending the partial would
// strand the fresh client's parser mid-escape (truncated OSC title, mid-rune
// U+FFFD). The partial is appended ONLY on the degraded/raw path, where the
// continuation genuinely follows over the live byte stream.
func TestModelDriven_HealthyAttachSnapshotHasNoPendingInput(t *testing.T) {
	s := newBareSession("sid-md-pending", "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 200)
	s.model = m
	s.serializer = ser
	s.emitter = model.NewDiffEmitter()

	// Feed printable text plus a trailing INCOMPLETE CSI so the model buffers a
	// non-empty pending partial (an unterminated "\x1b[1;5").
	s.pumpStep([]byte("hello\x1b[1;5"))
	require.NotEmpty(t, s.model.PendingInput(), "test setup: model must hold a pending partial")

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	f, ok := waitFrame(t, ch)
	require.True(t, ok, "attach must deliver a snapshot")
	require.True(t, f.Snapshot, "healthy attach frame must be a snapshot")

	s.mu.Lock()
	want := ser.Serialize(m)
	pending := append([]byte(nil), s.model.PendingInput()...)
	s.mu.Unlock()

	require.NotEmpty(t, pending, "test setup: partial must still be buffered at attach time")
	assert.Equal(t, string(want), string(f.Data),
		"healthy attach snapshot must equal serializer output with no pending partial appended")
	assert.NotContains(t, string(f.Data), string(pending),
		"the buffered mid-sequence partial must never appear in a healthy attach snapshot")
}

func TestModelDriven_DegradedFallsBackToRaw(t *testing.T) {
	s, err := New("sid-md-deg", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 200)
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
	collectUntil(t, ch, "RAW-FALLBACK-7") // the arrival IS the assertion: raw streaming survived
}

// TestModelDriven_ResizeInvalidatesEmitterForcingNextKeyframe proves Resize
// invalidates the diff emitter (spec: a resize can never be expressed as an
// absolute-addressed diff), so the very next model-derived frame after a
// resize is a Snapshot keyframe rather than an incremental diff.
func TestModelDriven_ResizeInvalidatesEmitterForcingNextKeyframe(t *testing.T) {
	s, err := New("sid-md-resize", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	// Drain the initial attach snapshot and let the emitter settle (Prime'd).
	_, ok := waitFrame(t, ch)
	require.True(t, ok, "attach must deliver an initial snapshot")

	require.NoError(t, s.Resize(100, 40))

	require.NoError(t, s.Write([]byte("echo POST-RESIZE\n")))
	f, ok := waitFrame(t, ch)
	require.True(t, ok, "post-resize output must produce a frame")
	assert.True(t, f.Snapshot, "the first frame emitted after Resize must be a keyframe (emitter invalidated)")
	// The resize keyframe (the freshly-resized blank screen) can flush before the
	// shell's echo output is parsed and rendered, so the marker lands in this keyframe
	// or a later frame depending on how the PTY chunks the write. Keep taking frames
	// until it shows rather than asserting it rode the very first keyframe.
	if !strings.Contains(string(f.Data), "POST-RESIZE") {
		collectUntil(t, ch, "POST-RESIZE")
	}
}

// TestModelDriven_AttachFlushesPendingDeltaBeforeRebasing proves Attach flushes
// any pending delta to EXISTING clients via emitFrameLocked before serializing the
// fresh-attach snapshot and priming the emitter — so a delta accumulated behind an
// unfired trailing frame-clock timer between the last emit and this attach is never
// silently dropped for clients that were already attached (M5d).
//
// Built directly on newBareSession + pumpStep (like the other timing-sensitive
// model-driven tests) so the pre-attach write can be pinned inside the coalesce
// window deterministically — a live PTY's own chunking would race the assertion.
// It asserts the flushed frame's CONTENT (the pre-attach write's text), not just
// that a frame arrived, and that the new client's snapshot already includes it.
func TestModelDriven_AttachFlushesPendingDeltaBeforeRebasing(t *testing.T) {
	s := newBareSession("sid-md-flush", "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 200)
	s.model = m
	s.serializer = ser
	s.emitter = model.NewDiffEmitter()

	// Freeze the frame clock: the coalescing decision below must be a fact, not a race
	// against an 8ms window that this test's own setup can lose under load.
	pinned := pinFrameClock(s)

	ch1, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch1)
	_, ok := waitFrame(t, ch1)
	require.True(t, ok, "attach must deliver an initial snapshot")

	// Cold-clock immediate emit stamps lastEmitAt.
	s.pumpStep([]byte("echo A\n"))
	_, ok = waitFrame(t, ch1)
	require.True(t, ok, "first chunk must emit immediately (cold clock)")

	// This chunk must only ARM the trailing timer, leaving its delta pending (unflushed) when
	// the second client attaches. Parking the clock makes that a fact rather than a race: the
	// chunk cannot take the immediate-emit branch, and the timer it arms cannot fire and flush
	// the delta before Attach gets the chance to — which is the whole thing under test.
	parkTrailingTimer(s, pinned)
	s.pumpStep([]byte("PENDING-DELTA-XYZ"))
	s.mu.Lock()
	armed := s.emitTimer != nil
	s.mu.Unlock()
	require.True(t, armed, "test setup: the second chunk must arm the trailing timer (pending delta)")

	// Second attach must flush that pending delta to the EXISTING client (ch1)
	// before rebasing the emitter for the new client (ch2).
	ch2, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch2)

	f1, ok := waitFrame(t, ch1)
	require.True(t, ok, "Attach must flush the pending delta to the existing client")
	assert.False(t, f1.Snapshot, "the flushed pending delta is an incremental diff, not a snapshot")
	assert.Contains(t, string(f1.Data), "PENDING-DELTA-XYZ",
		"the flushed delta frame must carry the pre-attach write's text, not silently drop it")

	// The new client's own snapshot must already reflect the pre-attach write.
	f2, ok := waitFrame(t, ch2)
	require.True(t, ok, "second attach must deliver a snapshot")
	assert.True(t, f2.Snapshot)
	assert.Contains(t, string(f2.Data), "PENDING-DELTA-XYZ",
		"the new client's attach snapshot must already include the pre-attach write")
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
	s.emitter = model.NewDiffEmitter()

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	// Drain the initial attach snapshot so it can't be mistaken for the fallback frame.
	_, ok := waitFrame(t, ch)
	require.True(t, ok, "attach must deliver an initial snapshot")

	// Arm the emit-path panic for exactly the next emitLocked call, then drive that
	// exact chunk through pumpStep directly.
	forceEmitPanicForTest(s)
	s.pumpStep([]byte("LOST-CHUNK-GUARD"))

	f, ok := waitFrame(t, ch)
	require.True(t, ok, "the chunk that triggered the emit-path panic must still reach the client via raw fallback, not be dropped")
	assert.Equal(t, "LOST-CHUNK-GUARD", string(f.Data),
		"the fallback frame must carry the triggering chunk's raw bytes verbatim")
	assert.False(t, f.Snapshot, "the fallback frame is a raw chunk, not a model snapshot")

	// The session must now be flipped to raw (modelPanics > 0) and keep streaming
	// normally through the pre-existing raw branch for every subsequent chunk.
	s.pumpStep([]byte("AFTER-FLIP"))
	f2, ok := waitFrame(t, ch)
	require.True(t, ok, "session must keep streaming raw after the flip")
	assert.Equal(t, "AFTER-FLIP", string(f2.Data))
}

// TestModelDriven_BurstCoalescesFrames proves the Task 7 adaptive frame clock: a burst of
// chunks arriving faster than minEmitInterval coalesces onto ONE pending trailing frame — the
// client is never sent one frame per chunk.
//
// The entire difficulty of this test is establishing its own premise, and every previous
// version tried to establish it by BEING FAST. It cannot be done that way, and the failures
// were not flakes — they were the test measuring its own runtime:
//
//   - v1 drove a live `seq 1 200` and asserted frames < 100. A descheduled pump leaves the
//     immediate-emit branch true for many chunks in one logical burst, so the count inflated
//     under CPU contention.
//   - v2 drove pumpStep directly and re-stamped lastEmitAt = time.Now() before each chunk. But
//     scheduleEmitLocked reads the WALL CLOCK, and under -race the 50 pumpStep cycles take
//     9-19ms against an 8ms window: a chunk crosses the boundary, emits immediately, and shows
//     up as an extra frame.
//   - v3 pinned the clock, which fixed the coalesce-vs-emit DECISION — but chunk 1 still armed
//     a real 8ms time.AfterFunc, and that timer fired MID-LOOP while chunks 2-49 were still
//     landing, so a later chunk armed a second timer and produced a second frame. (~1 in 40
//     under a full parallel -race run.)
//
// So the clock is pinned AND lastEmitAt is parked in the future, which makes the armed timer's
// delay effectively infinite: it cannot fire while the burst is in flight, on any machine, at
// any load. The coalescing claim is then a SYNCHRONOUS one — fan-out to a client happens under
// s.mu inside pumpStep, so if any chunk had emitted, its frame would already be sitting in the
// channel. An empty channel after 50 chunks IS the property, stated without a clock.
//
// The trailing timer actually FIRING is not this test's job: TestModelDriven_TrailingTimerFlushesFinalState
// covers that, and covers it by blocking on the frame rather than racing it.
func TestModelDriven_BurstCoalescesFrames(t *testing.T) {
	s := newBareSession("sid-md-burst", "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 2000)
	s.model = m
	s.serializer = ser
	s.emitter = model.NewDiffEmitter()

	// Freeze this session's frame clock.
	pinned := pinFrameClock(s)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	_, ok := waitFrame(t, ch)
	require.True(t, ok, "attach must deliver an initial snapshot")

	// Park the last-emit stamp in the FUTURE. Two consequences, both wanted: every chunk in the
	// burst takes the coalescing branch (none can emit synchronously), and the trailing timer
	// chunk 1 arms cannot fire during the burst and steal a flush out from under the test.
	parkTrailingTimer(s, pinned)

	// The burst. The real scheduleEmitLocked runs unmodified for all 50: the first arms the
	// trailing timer, the other 49 short-circuit on it being already armed.
	const burstChunks = 50
	for i := 0; i < burstChunks; i++ {
		s.pumpStep([]byte(fmt.Sprintf("line%d\n", i)))
	}

	s.mu.Lock()
	armed := s.emitTimer != nil
	s.mu.Unlock()
	require.True(t, armed, "the burst must have coalesced onto a trailing timer")

	// THE ASSERTION. Fan-out happens synchronously under s.mu inside pumpStep, so a chunk that
	// emitted would have already put its frame in this channel. Zero frames for 50 chunks is
	// the coalescing property — checked as a fact, not watched for over a window.
	require.Empty(t, ch, "a burst inside one interval must emit NO per-chunk frames; all 50 coalesce")

	// Now flush the single pending delta through the production path (the same one Attach and
	// Detach use) and prove the whole burst arrives as exactly ONE frame carrying the
	// accumulated screen — not a fragment, and not fifty.
	s.mu.Lock()
	s.flushPendingEmitLocked()
	s.mu.Unlock()

	f, ok := waitFrame(t, ch)
	require.True(t, ok, "the pending trailing delta must flush as a frame")
	require.Contains(t, string(f.Data), "line0", "the coalesced frame carries the whole burst")
	require.Contains(t, string(f.Data), fmt.Sprintf("line%d", burstChunks-1),
		"the coalesced frame carries the whole burst, through its last chunk")
	require.Empty(t, ch, "the coalesced burst yields exactly ONE frame, never a second")
}

// TestModelDriven_BurstOverLivePTYDeliversAllOutput is a live-PTY smoke test
// kept alongside the deterministic coalescing test above: it asserts only a
// contention-proof property (all 200 lines eventually arrive), never a frame
// COUNT, so it cannot flake under CPU pressure the way the old frames < 100
// assertion did while still exercising the real spawn → pump → pumpStep path
// end to end against a live shell.
func TestModelDriven_BurstOverLivePTYDeliversAllOutput(t *testing.T) {
	s, err := New("sid-md-burst-live", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 2000)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	quiesce(t, s, ch) // at the prompt, attach snapshot drained — not "500ms should be enough"

	// The end-of-burst marker is printed from TWO printf arguments, so the joined token
	// "SEQEND" can only appear in the command's OUTPUT — never in the PTY's echo of the typed
	// line, which shows "SEQ END". PTY output is ordered, so once SEQEND is on the wire every
	// line the burst produced has already been fanned out. That, not a 5s drain, is what makes
	// "all the output arrived" a checkable statement.
	require.NoError(t, s.Write([]byte("seq 1 200; printf '%s%s\\n' SEQ END\n")))
	data := collectUntil(t, ch, "SEQEND")
	assert.Contains(t, data, "200", "burst output must fully arrive regardless of coalescing frame count")
}

// TestModelDriven_TrailingTimerFlushesFinalState proves output that arrives
// entirely inside one 8ms coalesce window still reaches the client: the
// trailing timer must fire and flush it rather than losing it.
func TestModelDriven_TrailingTimerFlushesFinalState(t *testing.T) {
	s, err := New("sid-md-trail", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	quiesce(t, s, ch)

	// The marker's ARRIVAL is the assertion — whether it rides an immediate emit or the trailing
	// timer, it must not be lost. Blocking on it says exactly that, and says it without betting
	// on 2 seconds being enough.
	require.NoError(t, s.Write([]byte("echo TRAILING-EDGE-OK\n")))
	collectUntil(t, ch, "TRAILING-EDGE-OK")
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
	s.emitter = model.NewDiffEmitter()

	// Freeze the frame clock: the coalescing decision below must be a fact, not a race
	// against an 8ms window that this test's own setup can lose under load.
	pinned := pinFrameClock(s)

	ch1, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch1)
	_, ok := waitFrame(t, ch1)
	require.True(t, ok, "attach must deliver an initial snapshot")

	// The clock starts cold (lastEmitAt is zero), so this first chunk emits
	// immediately and also stamps lastEmitAt to "now".
	s.pumpStep([]byte("echo A\n"))
	_, ok = waitFrame(t, ch1)
	require.True(t, ok, "first chunk must emit immediately (cold clock)")

	// This second chunk lands inside the 8ms window and must only ARM the
	// trailing timer rather than emit synchronously.
	// Park the clock so this second chunk is inside the window BY CONSTRUCTION, and so the
	// timer it arms cannot fire before the assertion below attributes the disarm to the code
	// under test.
	parkTrailingTimer(s, pinned)
	s.pumpStep([]byte("echo B\n"))
	s.mu.Lock()
	armed := s.emitTimer != nil
	s.mu.Unlock()
	require.True(t, armed, "test setup: the second chunk must coalesce onto the trailing timer")

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
	_, ok = waitFrame(t, ch1)
	require.True(t, ok, "Attach must flush the pending delta to the existing client")

	f2, ok := waitFrame(t, ch2)
	require.True(t, ok, "second attach must deliver a snapshot")
	assert.True(t, f2.Snapshot)

	// The disarmed timer must never separately fire afterward and duplicate the flush already
	// delivered above. Proved by a SEQUENCE BARRIER rather than by watching ch1 for 50ms: force
	// the clock cold and pump a NEW, identifiable chunk, which must emit immediately. ch1 is
	// FIFO, so a stale timer's duplicate diff (built off the pre-attach base — the corruption
	// this test exists to prevent) could only sit AHEAD of that new frame. Receiving the new
	// chunk's text as the very next frame therefore proves no such duplicate was ever emitted —
	// and it does so without waiting out a window in which "nothing happened yet" was only ever
	// evidence that nothing had happened YET.
	coldFrameClock(s) // the next chunk emits synchronously
	s.pumpStep([]byte("POST-ATTACH-BARRIER"))

	next, ok := waitFrame(t, ch1)
	require.True(t, ok, "the post-attach chunk must reach the existing client")
	assert.Contains(t, string(next.Data), "POST-ATTACH-BARRIER",
		"the next frame on ch1 must be the post-attach chunk: a stale trailing timer must not have "+
			"fired a duplicate delta ahead of it")
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
	s.emitter = model.NewDiffEmitter()

	// Freeze the frame clock: the coalescing decision below must be a fact, not a race
	// against an 8ms window that this test's own setup can lose under load.
	pinned := pinFrameClock(s)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	_, ok := waitFrame(t, ch)
	require.True(t, ok, "attach must deliver an initial snapshot")

	s.pumpStep([]byte("echo A\n"))
	_, ok = waitFrame(t, ch)
	require.True(t, ok, "first chunk must emit immediately (cold clock)")

	// Park the clock so this second chunk is inside the window BY CONSTRUCTION, and so the
	// timer it arms cannot fire before the assertion below attributes the disarm to the code
	// under test.
	parkTrailingTimer(s, pinned)
	s.pumpStep([]byte("echo B\n"))
	s.mu.Lock()
	armed := s.emitTimer != nil
	s.mu.Unlock()
	require.True(t, armed, "test setup: the second chunk must coalesce onto the trailing timer")

	ok = s.Resync()
	require.True(t, ok, "Resync must emit a keyframe for a non-idle bare session")

	s.mu.Lock()
	stillArmed := s.emitTimer != nil
	s.mu.Unlock()
	assert.False(t, stillArmed, "Resync must disarm the stale trailing timer")

	f, ok := waitFrame(t, ch)
	require.True(t, ok, "Resync must deliver its keyframe")
	assert.True(t, f.Snapshot, "Resync's frame must be a keyframe")

	// The disarmed timer must never separately fire afterward and deliver a second, redundant
	// frame. Same SEQUENCE BARRIER as the Attach case: force the clock cold and pump a new,
	// identifiable chunk that must emit immediately. The client channel is FIFO, so a stale
	// timer's redundant frame could only precede it — receiving the new chunk's text as the very
	// next frame proves no such frame was emitted, with no window to wait out.
	coldFrameClock(s) // the next chunk emits synchronously
	s.pumpStep([]byte("POST-RESYNC-BARRIER"))

	next, ok := waitFrame(t, ch)
	require.True(t, ok, "the post-resync chunk must reach the client")
	assert.Contains(t, string(next.Data), "POST-RESYNC-BARRIER",
		"the next frame must be the post-resync chunk: the stale trailing timer must not have "+
			"fired a redundant frame ahead of it")
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
	s.emitter = model.NewDiffEmitter()

	// Freeze the frame clock: the coalescing decision below must be a fact, not a race
	// against an 8ms window that this test's own setup can lose under load.
	pinned := pinFrameClock(s)

	// Attach a client BEFORE arming the timer so there is a channel to drain
	// and count frames on after teardown.
	ch, err := s.Attach()
	require.NoError(t, err)
	_, ok := waitFrame(t, ch)
	require.True(t, ok, "attach must deliver an initial snapshot")

	// Cold-clock immediate emit, then a second chunk inside the window arms
	// the trailing timer — same two-step pattern as the Attach/Resync
	// disarm tests above.
	s.pumpStep([]byte("echo A\n"))
	_, ok = waitFrame(t, ch)
	require.True(t, ok, "first chunk must emit immediately (cold clock)")

	// Park the clock so this second chunk is inside the window BY CONSTRUCTION, and so the
	// timer it arms cannot fire before the assertion below attributes the disarm to the code
	// under test.
	parkTrailingTimer(s, pinned)
	s.pumpStep([]byte("echo B\n"))
	s.mu.Lock()
	armed := s.emitTimer != nil
	s.mu.Unlock()
	require.True(t, armed, "test setup: the second chunk must coalesce onto the trailing timer")

	// Kill on a placeholder-shaped bare session (ptmx == nil) takes the
	// direct shutdown() branch, exercising the exact guard under test: a
	// still-armed timer at teardown time.
	require.NotPanics(t, func() { s.Kill() })

	// Kill's shutdown closes s.done; block on that real signal.
	<-s.Done()

	// (1) The timer was STOPPED, stated directly rather than inferred from a quiet window:
	// shutdown calls stopEmitTimerLocked under the same s.mu hold that closes s.done, and that
	// nils the handle. Dropping the stop call — the regression this test guards — leaves a
	// non-nil handle here, and is caught instantly, on any machine, at any speed.
	s.mu.Lock()
	stillArmed := s.emitTimer != nil
	s.mu.Unlock()
	assert.False(t, stillArmed, "teardown must stop the armed trailing timer")

	// (2) No frame reached the client. shutdown closes every client's channel, so ranging to the
	// close is a complete, terminating read of everything that was ever delivered — the close is
	// the real signal that no further frame can exist. The old version instead drained for
	// 2×minEmitInterval and hoped the stopped timer's window had passed.
	frames := 0
	for f := range ch {
		frames++
		t.Logf("unexpected frame after teardown: %+v", f)
	}
	assert.Equal(t, 0, frames, "no frame may arrive after teardown (trailing timer must have been stopped)")
}

// TestModelDriven_CPRQueryAnsweredToPTY proves the daemon answers a cursor-position-report
// query (ESC[6n) from the model (spec §3.8): the client xterm
// never sees the query in this mode, so without the session's response sink writing the
// model's synthesized reply back to the PTY master, an app reading the reply from stdin
// (here, the shell's `read`) would hang/time out.
func TestModelDriven_CPRQueryAnsweredToPTY(t *testing.T) {
	// The read vehicle below needs `read -s -t 3 -d R`, which are bash extensions:
	// on a dash /bin/sh (Linux CI) they error out ("read: Illegal option -s") and the
	// shell falls through to NOANSWER before the daemon's reply can ever matter. Drive
	// the round-trip through bash explicitly so the test exercises the response sink,
	// not the host shell's read dialect.
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable; CPR read vehicle requires bash (read -s/-d/-t)")
	}
	s, err := New("sid-md-cpr", bash, t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	// No settle step: the script below is queued in the tty's input buffer and executed when
	// the shell gets to it, and its own printed verdict — not a drained interval — is what this
	// test blocks on. (This session's shell is bash, whose interactive startup reads ~/.bashrc
	// regardless of BASH_ENV, so the pinned PS1 is not a reliable readiness signal here anyway;
	// the script's markers are, and they are what the assertions are about.)
	//
	// `read -s -t 3 REPLY` after emitting ESC[6n: the shell captures whatever
	// comes back on stdin. If the daemon answers, REPLY holds the CPR and the
	// marker prints; with no answer, read times out and prints NOANSWER.
	//
	// TIMING-BY-SUBJECT (fixture-side): the `-t 3` belongs to the SHELL, not to the test's
	// synchronisation. It is the vehicle that turns "the daemon never answered" into an
	// OBSERVABLE verdict (NOANSWER) instead of a `read` that blocks forever — i.e. it is what
	// gives the failing case a printed outcome to assert on. The test itself waits on that
	// verdict, whichever way it goes, with no deadline of its own.
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

	// The if/else prints EXACTLY ONE of the two markers, so blocking until either appears is a
	// closed question with a real signal — and it reports the failure ("NOANSWER arrived")
	// precisely, instead of a 5s drain reporting the absence of the marker it wanted.
	data := collectUntil(t, ch, "CPR-ANSWERED", "NOANSWER")
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
	s, err := New("sid-md-sink-flip", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 200)
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

	// Drive the flip through the modelEmitHealthyLocked latch (pumpStep -> writeModelLocked
	// -> modelEmitHealthyLocked), the same way TestModelDriven_DegradedFallsBackToRaw does.
	// The very chunk that flips the session is also the one the (now) raw path fans out, so the
	// marker's ARRIVAL at the client is proof the flip has already latched — a real signal that
	// makes the assertions below valid, where the old 3s drain merely assumed it.
	require.NoError(t, s.Write([]byte("echo SINK-FLIP-TRIGGER\n")))
	collectUntil(t, ch, "SINK-FLIP-TRIGGER")

	s.mu.Lock()
	stillInstalled := model.ResponseSinkInstalledForTest(s.model)
	fellBack := s.modelDrivenFellBack
	s.mu.Unlock()
	require.True(t, fellBack, "test setup: the degraded flip must have latched")
	assert.False(t, stillInstalled, "the response sink must be uninstalled once the session flips to raw fallback")
}
