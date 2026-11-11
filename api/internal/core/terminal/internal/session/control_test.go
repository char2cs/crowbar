//go:build !windows

package session

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

// attachQuiet starts /bin/sh at 50x10, attaches, prints a URL that wraps at 50 columns and
// drains every frame, so the test sees only what its own Resize causes.
func attachQuiet(t *testing.T, id string) (*Session, <-chan OutputFrame) {
	t.Helper()
	s, err := New(t.Context(), id, "/bin/sh", t.TempDir(), "", testEnv(), 50, 10, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	t.Cleanup(func() { s.Detach(ch) })
	quiesce(t, s, ch)
	runShell(t, s, "printf '%s\\n' https://example.com/"+strings.Repeat("x", 45))
	s.mu.Lock()
	s.flushPendingEmitLocked()
	s.mu.Unlock()
	drainFrames(ch)
	return s, ch
}

// assertRebuilds replays a keyframe into a fresh client at the session's size and requires it
// to reproduce the session's model, cursor included.
func assertRebuilds(t *testing.T, s *Session, f OutputFrame, cols, rows int) {
	t.Helper()
	require.True(t, f.Snapshot, "a resize frame is a keyframe")
	replay, ser := model.New(cols, rows, 200)
	defer replay.Close()
	replay.Write(f.Data)
	s.mu.Lock()
	want := ser.Serialize(s.model)
	s.mu.Unlock()
	assert.Equal(t, string(want), string(ser.Serialize(replay)), "the keyframe rebuilds the reflowed screen and cursor")
}

// takeFrames returns every frame already queued on ch.
func takeFrames(ch <-chan OutputFrame) []OutputFrame {
	var frames []OutputFrame
	for len(ch) > 0 {
		frames = append(frames, <-ch)
	}
	return frames
}

// The resize tests run in a synctest bubble, so the settle timer a resize arms runs on a fake
// clock; the PTY session is started outside it, where its I/O can block.

// stopTimerInBubble stops, inside the bubble, a timer a test leaves armed: stopping a bubble's
// timer from outside it (the session's own cleanup) is fatal.
func stopTimerInBubble(t *testing.T, s *Session) {
	t.Helper()
	t.Cleanup(func() {
		s.mu.Lock()
		s.stopEmitTimerLocked()
		s.mu.Unlock()
	})
}

// TestModelDriven_ResizeSendsTheReflowedKeyframe pins a reload at a new width: the attach
// snapshot was laid out at the old width, and a shell idle at its prompt prints nothing on
// SIGWINCH, so the resize must send the keyframe or the client keeps a stale cursor.
func TestModelDriven_ResizeSendsTheReflowedKeyframe(t *testing.T) {
	s, ch := attachQuiet(t, "sid-md-resize")
	synctest.Test(t, func(t *testing.T) {
		stopTimerInBubble(t, s)
		require.NoError(t, s.Resize(74, 10))
		require.Empty(t, ch, "the keyframe waits for the resizes to settle")

		time.Sleep(resizeSettle)
		synctest.Wait()
		frames := takeFrames(ch)
		require.Len(t, frames, 1, "the settled resize sends its keyframe with no app output")
		assertRebuilds(t, s, frames[0], 74, 10)
	})
}

// A 60 Hz window drag resizes every 16ms while the app redraws on each SIGWINCH; the burst
// collapses into one keyframe of the last size, sent after the last resize.
func TestModelDriven_ResizeBurstSendsOneKeyframe(t *testing.T) {
	s, ch := attachQuiet(t, "sid-md-resize-burst")
	synctest.Test(t, func(t *testing.T) {
		stopTimerInBubble(t, s)
		coldFrameClock(s) // output outside a settle window would emit at once
		for cols := uint16(51); cols <= 74; cols++ {
			require.NoError(t, s.Resize(cols, 10))
			s.PumpChunkForTest([]byte("."))
			time.Sleep(16 * time.Millisecond)
			synctest.Wait()
			require.Empty(t, ch, "no frame while the drag is under way (%d cols)", cols)
		}

		time.Sleep(resizeSettle)
		synctest.Wait()
		frames := takeFrames(ch)
		require.Len(t, frames, 1, "one keyframe for the whole burst")
		assertRebuilds(t, s, frames[0], 74, 10)
	})
}

// A resize to the size the PTY already has changes nothing, so it sends nothing.
func TestModelDriven_SameSizeResizeSendsNothing(t *testing.T) {
	s, ch := attachQuiet(t, "sid-md-resize-same")
	synctest.Test(t, func(t *testing.T) {
		stopTimerInBubble(t, s)
		require.NoError(t, s.Resize(50, 10))

		time.Sleep(resizeSettle)
		synctest.Wait()
		require.Empty(t, ch, "a same-size resize sends no frame")
	})
}

// With no client attached a resize renders nothing; the next attach serializes and primes.
func TestModelDriven_ResizeWithNoClientSchedulesNothing(t *testing.T) {
	s, ch := attachQuiet(t, "sid-md-resize-none")
	s.Detach(ch)
	synctest.Test(t, func(t *testing.T) {
		stopTimerInBubble(t, s)
		require.NoError(t, s.Resize(74, 10))

		s.mu.Lock()
		defer s.mu.Unlock()
		require.Nil(t, s.emitTimer, "no keyframe is scheduled for nobody")
	})
}
