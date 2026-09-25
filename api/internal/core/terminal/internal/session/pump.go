package session

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// foregroundSampleInterval debounces the per-chunk foreground-process-group sample so
// the TIOCGPGRP ioctl (and the app-death-edge model teardown) stays off the hot path
// while still converging within ~¼ s of an app exiting (§11.1 sampling site #1).
const foregroundSampleInterval = 250 * time.Millisecond

// minEmitInterval is the model-driven frame clock (spec §3.3): interactive
// deltas emit immediately; bursts coalesce to at most one frame per interval.
const minEmitInterval = 8 * time.Millisecond

// pumpStep is the production critical section for one PTY output chunk. Under s.mu it
// writes the chunk into the model, lets the frame clock emit the model-derived frame, and
// then — last, debounced — samples the foreground process group so neither the ioctl nor
// the app-death teardown can ever precede or delay the emit. OSC 7 is scanned outside the
// lock on the freshly-owned chunk.
func (s *Session) pumpStep(chunk []byte) {
	path, ok := parseLastOSC7(chunk)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		s.cwd = path
	}
	// Model-driven (spec §3.1): the model is written FIRST and clients receive the
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

// notifyPumpLocked publishes "the pump has fully processed another chunk" on the
// pumpNotify seam. It runs LAST in pumpStep's critical section, so a woken waiter is
// guaranteed to observe every effect of that chunk — the model write, the emit, and
// s.dirty — as soon as it can re-acquire s.mu.
//
// The send is non-blocking onto a 1-buffered channel, making the signal a coalescing
// edge ("something happened since you last looked") rather than a queue: the pump can
// never block on, or be paced by, whether anyone is listening, so production behaviour
// with no listener is byte-for-byte what it was before the seam existed. Caller holds
// s.mu.
func (s *Session) notifyPumpLocked() {
	select {
	case s.pumpNotify <- struct{}{}:
	default:
	}
}

// resetModelLocked is the §8.5 backstops' recovery. A model method escaped a panic, so
// the model's state can no longer be trusted: rebuild a fresh model at the PTY's current
// size (answering device queries through the same sink) and invalidate the diff base, so
// the next frame every client gets is a keyframe of the new ground state rather than a
// diff off a corrupt one. The screen restarts blank; the session keeps running. Caller
// holds s.mu.
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
			defer func() { _ = recover() }()
			old.Close()
		}()
	}
	_, _ = fmt.Fprintf(os.Stderr, "terminal: session %s: model panic recovered, screen model rebuilt\n", s.id)
}

// scheduleEmitLocked implements the adaptive frame clock (spec §3.3, Task 7):
// when at least minEmitInterval has elapsed since the last emit, it emits
// immediately (so interactive echo is never batched); otherwise it arms — or
// leaves armed — a single trailing timer that fires exactly at the interval
// boundary, coalescing an arbitrarily long burst of chunks into one frame per
// interval. Caller holds s.mu. Returns whether it emitted synchronously
// (false means a trailing timer now owns the pending delta).
func (s *Session) scheduleEmitLocked() bool {
	if s.now().Sub(s.lastEmitAt) >= minEmitInterval {
		s.lastEmitAt = s.now()
		s.emitFrameLocked()
		return true
	}
	if s.emitTimer != nil {
		return false // trailing emit already armed
	}
	delay := minEmitInterval - s.now().Sub(s.lastEmitAt)
	// Capture the timer's own identity so a stale, lock-blocked callback can
	// only clear ITS OWN handle: if an explicit stop (stopEmitTimerLocked) had
	// already nil'd s.emitTimer and a NEWER timer t2 was armed before this
	// callback got the lock, comparing against the captured local t (rather
	// than unconditionally nil-ing s.emitTimer) leaves t2's handle intact —
	// t2 still fires safely either way, but this keeps stopEmitTimerLocked
	// able to cancel it during the gap instead of losing track of it.
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
		// emitFrameLocked → emitLocked → DiffEmitter.Emit is safe to invoke
		// even if a newer immediate emit already flushed this same delta: Emit
		// is idempotent against its own primed base (an empty diff yields no
		// frame), so a harmless double-emit race here never double-delivers
		// visible output to clients.
		s.emitFrameLocked()
	})
	// Assign under the already-held s.mu (not inside the callback closure)
	// so the field update happens-before any concurrent read of s.emitTimer
	// under the lock; the callback only ever fires after AfterFunc returns
	// with delay > 0, but this ordering removes any reliance on that timing.
	s.emitTimer = t
	return false
}

// stopEmitTimerLocked cancels an armed trailing emit timer, if any, without
// running the emit it would otherwise have performed. Used at lifecycle
// boundaries (teardown) that either perform their own equivalent
// emit or no longer need one, so a stale timer can never fire a redundant or
// out-of-order frame afterward. Caller holds s.mu.
func (s *Session) stopEmitTimerLocked() {
	if s.emitTimer != nil {
		s.emitTimer.Stop()
		s.emitTimer = nil
	}
}

// flushPendingEmitLocked cancels an armed trailing emit timer and, only if
// one was armed (i.e. a delta is genuinely waiting), runs the emit
// immediately in its place. Used at boundaries — Attach's flush-then-serialize
// is the current caller — where client-visible state is about to be read or
// rebased and a delta accumulated under the trailing timer must reach
// existing clients first, rather than being silently superseded. A no-op
// when no timer is armed (the last emit was already synchronous). Caller
// holds s.mu.
func (s *Session) flushPendingEmitLocked() {
	if s.emitTimer == nil {
		return
	}
	s.stopEmitTimerLocked()
	s.lastEmitAt = s.now()
	s.emitFrameLocked()
}

// emitFrameLocked derives one frame from the model and fans it out: a diff
// frame normally, a snapshot keyframe when the emitter demands one (see
// model.DiffEmitter.Emit for the canonical, exhaustive keyframe-trigger list).
// Caller holds s.mu. Reached either synchronously from
// scheduleEmitLocked/flushPendingEmitLocked or directly by Attach's own
// forced keyframes, which always reflect the full current model state regardless
// of the frame clock.
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

// emitLocked / primeLocked wrap the emitter in the same §8.5 recover backstop as every
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

// writeModelLocked feeds a chunk into the model under a recover backstop. In production
// this is defence-in-depth: vtModel.Write recovers its own parse panics internally
// (recreateEmu). Should a Write ever escape one, the model is rebuilt rather than
// stranding s.mu or killing the session (§8.2). Caller holds s.mu.
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
// has a truthful keyframe to send (§8.5). Caller holds s.mu.
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
// escaping (§8.5). Caller holds s.mu.
func (s *Session) mutateModelLocked(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			s.resetModelLocked()
		}
	}()
	fn()
	// Every caller here reshapes or clears the visible grid (Resize, the
	// foreground-app teardown), so the screen an observer last read is stale even
	// though no byte arrived from the PTY. Bumped AFTER fn so a panicking mutation
	// — which the recover above swallows — does not claim a change it never made.
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
				_, _ = fmt.Fprintf(os.Stderr, "terminal: session %s: pump error: %v\n", s.id, err)
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

// parseLastOSC7 scans b for OSC 7 sequences of the form
//
//	ESC ] 7 ; file://[host]/path BEL
//
// and returns the decoded path from the last match found. Best-effort: partial sequences
// that span chunk boundaries are silently ignored.
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
