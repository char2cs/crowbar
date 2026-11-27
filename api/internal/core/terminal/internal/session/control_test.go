//go:build !windows

package session

import (
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

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
	parkShell(t, s)
	s.mu.Lock()
	s.flushPendingEmitLocked()
	s.mu.Unlock()
	drainFrames(ch)
	return s, ch
}

func parkShell(t *testing.T, s *Session) {
	t.Helper()
	n := markerSeq.Add(1)
	tok := fmt.Sprintf("PK%dON", n)
	require.NoError(t, s.Write([]byte(fmt.Sprintf("( printf '%%s%%s' PK%d ON; exec sleep 9999 )\n", n))))
	waitScreen(t, s, func(scr string) bool { return strings.Contains(scr, tok) }, "the parked child to announce itself")
}

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

func takeFrames(ch <-chan OutputFrame) []OutputFrame {
	var frames []OutputFrame
	for len(ch) > 0 {
		frames = append(frames, <-ch)
	}
	return frames
}

func stopTimerInBubble(t *testing.T, s *Session) {
	t.Helper()
	t.Cleanup(func() {
		s.mu.Lock()
		s.stopEmitTimerLocked()
		s.mu.Unlock()
	})
}

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

func TestModelDriven_ResizeBurstSendsOneKeyframe(t *testing.T) {
	s, ch := attachQuiet(t, "sid-md-resize-burst")
	synctest.Test(t, func(t *testing.T) {
		stopTimerInBubble(t, s)
		coldFrameClock(s)
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

func TestModelDriven_RedrawAfterResizeKeyframeIsADiff(t *testing.T) {
	s, ch := attachQuiet(t, "sid-md-resize-late")
	synctest.Test(t, func(t *testing.T) {
		stopTimerInBubble(t, s)
		require.NoError(t, s.Resize(74, 10))
		time.Sleep(resizeSettle)
		synctest.Wait()
		keyframe := takeFrames(ch)
		require.Len(t, keyframe, 1, "the settled resize sends its keyframe")

		time.Sleep(minEmitInterval)
		s.PumpChunkForTest([]byte("\r" + shellPrompt + "\x1b[K"))
		late := takeFrames(ch)
		require.Len(t, late, 1, "the late redraw emits at once")
		require.False(t, late[0].Snapshot, "a redraw after the keyframe is a diff on it")
		replay := OutputFrame{Data: append(keyframe[0].Data, late[0].Data...), Snapshot: true}
		assertRebuilds(t, s, replay, 74, 10)
	})
}

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
