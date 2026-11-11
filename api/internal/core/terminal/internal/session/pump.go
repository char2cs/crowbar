package session

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// foregroundSampleInterval debounces the per-chunk foreground-process-group sample so
// the TIOCGPGRP ioctl (and the app-death-edge model teardown) stays off the hot path
// while still converging within ~¼ s of an app exiting.
const foregroundSampleInterval = 250 * time.Millisecond

// minEmitInterval is the model-driven frame clock: interactive
// deltas emit immediately; bursts coalesce to at most one frame per interval.
const minEmitInterval = 8 * time.Millisecond

// resizeSettle is how long a resize waits for the next one before its keyframe goes out.
// It spans three dropped frames of a 60 Hz window drag, while a reloaded client that
// re-attached at the old width is stale for only this long.
const resizeSettle = 50 * time.Millisecond

// pumpStep is the critical section for one PTY chunk: model write, frame emit, then (last, debounced)
// the foreground sample, so neither can delay the emit. OSC 7 is scanned outside the lock.
func (s *Session) pumpStep(chunk []byte) {
	path, ok := parseLastOSC7(chunk)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		s.cwd = path
	}
	// Model-driven: the model is written FIRST and clients receive the
	// model-derived frame the frame clock schedules for it.
	s.writeModelLocked(chunk)
	s.scheduleEmitLocked()
	s.dirty = true
	if now := time.Now(); now.Sub(s.lastFgSampleAt) >= foregroundSampleInterval {
		s.lastFgSampleAt = now
		s.sampleForegroundLocked()
	}
	s.notifyPumpLocked()
}

// notifyPumpLocked signals pumpNotify last in pumpStep, so a waiter sees every effect of the chunk. The
// send is non-blocking onto a 1-buffered channel: a coalescing edge the pump is never paced by.
// Caller holds s.mu.
func (s *Session) notifyPumpLocked() {
	select {
	case s.pumpNotify <- struct{}{}:
	default:
	}
}

// resetModelLocked rebuilds the model after a method escaped a panic and invalidates the diff base, so
// clients get a keyframe of the new blank state rather than a diff off a corrupt one. Caller holds s.mu.
func (s *Session) resetModelLocked() {
	s.modelPanics++
	old := s.model
	s.model, s.serializer = s.newModel(s.cols, s.rows, s.scrollback)
	if s.replySink != nil {
		s.model.SetResponseSink(s.replySink)
	}
	s.emitter.Invalidate()
	s.dirty = true
	s.lastBlob = nil
	s.screenGen++
	if old != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("terminal: closing the corrupt screen model panicked", "component", "terminal", "session", s.id, "panic", r)
				}
			}()
			old.Close()
		}()
	}
	slog.Error("terminal: model panic recovered; screen model rebuilt", "component", "terminal", "session", s.id)
}

// scheduleEmitLocked is the adaptive frame clock: emit at once when minEmitInterval has passed (echo is
// never batched), else arm one trailing timer at the boundary so a burst coalesces. Caller holds s.mu.
func (s *Session) scheduleEmitLocked() {
	if s.emitTimer != nil {
		return // trailing emit already armed
	}
	if s.now().Sub(s.lastEmitAt) >= minEmitInterval {
		s.lastEmitAt = s.now()
		s.emitFrameLocked()
		return
	}
	s.armEmitTimerLocked(minEmitInterval - s.now().Sub(s.lastEmitAt))
}

// settleEmitLocked (re)arms the trailing emit resizeSettle from now, so a burst of resizes
// sends one keyframe after the last of them. Caller holds s.mu.
func (s *Session) settleEmitLocked() {
	s.stopEmitTimerLocked()
	s.armEmitTimerLocked(resizeSettle)
}

// armEmitTimerLocked arms the single trailing emit timer to fire after delay. Caller holds s.mu
// and has checked that no timer is armed.
func (s *Session) armEmitTimerLocked(
	delay time.Duration,
) {
	// Capture the timer's identity so a stale, lock-blocked callback clears only ITS OWN handle, never a
	// newer timer that stopEmitTimerLocked must still be able to cancel.
	var t *time.Timer
	t = time.AfterFunc(delay, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.emitTimer == t {
			s.emitTimer = nil
		}
		select {
		case <-s.done:
			return // session tore down while the timer was in flight
		default:
		}
		s.lastEmitAt = s.now()
		// Safe even if an immediate emit already flushed this delta: Emit is idempotent against its primed base.
		s.emitFrameLocked()
	})
	// Assigned under s.mu, not in the callback, so the write happens-before any locked read of emitTimer.
	s.emitTimer = t
}

// stopEmitTimerLocked cancels an armed trailing emit without running it, at teardown, so a stale timer
// never fires an out-of-order frame. Caller holds s.mu.
func (s *Session) stopEmitTimerLocked() {
	if s.emitTimer != nil {
		s.emitTimer.Stop()
		s.emitTimer = nil
	}
}

// flushPendingEmitLocked runs an armed trailing emit immediately, so a pending delta reaches clients
// before Attach rebases their state; a no-op when none is armed. Caller holds s.mu.
func (s *Session) flushPendingEmitLocked() {
	if s.emitTimer == nil {
		return
	}
	s.stopEmitTimerLocked()
	s.lastEmitAt = s.now()
	s.emitFrameLocked()
}

// emitFrameLocked derives one frame from the model and fans it out: a diff, or a keyframe when the
// emitter demands one (see model.DiffEmitter.Emit). Caller holds s.mu.
func (s *Session) emitFrameLocked() {
	data, needKeyframe := s.emitLocked()
	if needKeyframe {
		redraw := s.serializeLocked()
		if len(redraw) == 0 {
			return
		}
		s.fanOutFrameLocked(OutputFrame{SessionID: s.id, Data: redraw, Snapshot: true})
		s.primeLocked()
		return
	}
	if len(data) > 0 {
		s.fanOutFrameLocked(OutputFrame{SessionID: s.id, Data: data})
	}
}

// emitLocked / primeLocked wrap the emitter in the same recover backstop as every
// other model access. A panic rebuilds the model; emitLocked then asks for a keyframe of it.
func (s *Session) emitLocked() (data []byte, needKeyframe bool) {
	defer func() {
		if r := recover(); r != nil {
			s.resetModelLocked()
			data, needKeyframe = nil, true
		}
	}()
	return s.emitter.Emit(s.model)
}

func (s *Session) primeLocked() {
	defer func() {
		if r := recover(); r != nil {
			s.resetModelLocked()
		}
	}()
	s.emitter.Prime(s.model)
}

// writeModelLocked feeds a chunk to the model under a recover backstop: should a panic escape
// vtModel.Write's own recovery, the model is rebuilt rather than stranding s.mu. Caller holds s.mu.
func (s *Session) writeModelLocked(chunk []byte) {
	defer func() {
		if r := recover(); r != nil {
			s.resetModelLocked()
		}
	}()
	if s.model != nil {
		s.model.Write(chunk)
		s.screenGen++
	}
}

// serializeLocked renders the model's ground-state redraw. A Serialize that panics gets
// the model rebuilt and the (blank) fresh model serialized instead, so a caller always
// has a truthful keyframe to send. Caller holds s.mu.
func (s *Session) serializeLocked() []byte {
	if redraw, ok := s.trySerializeLocked(); ok {
		return redraw
	}
	s.resetModelLocked()
	redraw, _ := s.trySerializeLocked()
	return redraw
}

func (s *Session) trySerializeLocked() (redraw []byte, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			redraw, ok = nil, false
		}
	}()
	return s.serializer.Serialize(s.model), true
}

// mutateModelLocked runs a void model mutation under the same recover backstop as the
// Write/Serialize paths, so a Resize-drain or teardown panic rebuilds the model instead of
// escaping. Caller holds s.mu.
func (s *Session) mutateModelLocked(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			s.resetModelLocked()
		}
	}()
	fn()
	// Every caller reshapes or clears the grid, so the screen changed with no PTY byte; bumped after fn so a
	// swallowed panic claims no change.
	s.screenGen++
}

// pump reads PTY stdout and delivers each chunk via pumpStep.
func (s *Session) pump() {
	defer safego.Recover("terminal.session.pump")
	defer s.shutdown()

	s.mu.Lock()
	ptmx := s.ptmx
	s.mu.Unlock()
	if ptmx == nil {
		return
	}

	buf := make([]byte, 64*1024)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.pumpStep(chunk)
		}
		if err != nil {
			if !isNormalPTYClose(err) {
				slog.Error("terminal: pump error", "component", "terminal", "session", s.id, "err", err)
			}
			return
		}
	}
}

// isNormalPTYClose reports whether err is the expected error when the shell exits.
// On Linux the PTY master returns EIO; on macOS it returns io.EOF.
func isNormalPTYClose(
	err error,
) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.EIO
	}
	return false
}

// fanOutFrameLocked delivers an already-built frame to all currently attached clients.
// Clients whose channel is full are disconnected (drop-on-overflow). Caller must hold s.mu.
func (s *Session) fanOutFrameLocked(
	frame OutputFrame,
) {
	var overflow []*client
	for cl := range s.clients {
		select {
		case cl.send <- frame:
		default:
			overflow = append(overflow, cl)
		}
	}

	for _, cl := range overflow {
		delete(s.clients, cl)
		close(cl.send)
	}
}

// parseLastOSC7 returns the decoded path of the last `ESC ] 7 ; file://[host]/path BEL` in b.
// Best-effort: sequences split across chunks are ignored.
func parseLastOSC7(b []byte) (string, bool) {
	prefix := []byte("\x1b]7;")
	last := ""
	found := false

	for {
		idx := bytes.Index(b, prefix)
		if idx < 0 {
			break
		}
		b = b[idx+len(prefix):]

		end := -1
		for i := 0; i < len(b); i++ {
			if b[i] == '\x07' {
				end = i
				break
			}
			if b[i] == '\x1b' && i+1 < len(b) && b[i+1] == '\\' {
				end = i
				break
			}
		}
		if end < 0 {
			break
		}

		uri := string(b[:end])
		b = b[end+1:]

		if !strings.HasPrefix(uri, "file://") {
			continue
		}

		parsed, err := url.Parse(uri)
		if err != nil || parsed.Path == "" {
			continue
		}

		last = parsed.Path
		found = true
	}

	return last, found
}
