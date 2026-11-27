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

const foregroundSampleInterval = 250 * time.Millisecond

const minEmitInterval = 8 * time.Millisecond

const resizeSettle = 50 * time.Millisecond

func (s *Session) pumpStep(chunk []byte) {
	path, ok := parseLastOSC7(chunk)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		s.cwd = path
	}
	s.writeModelLocked(chunk)
	s.scheduleEmitLocked()
	s.dirty = true
	if now := time.Now(); now.Sub(s.lastFgSampleAt) >= foregroundSampleInterval {
		s.lastFgSampleAt = now
		s.sampleForegroundLocked()
	}
	s.notifyPumpLocked()
}

func (s *Session) notifyPumpLocked() {
	select {
	case s.pumpNotify <- struct{}{}:
	default:
	}
}

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

func (s *Session) scheduleEmitLocked() {
	if s.emitTimer != nil {
		return
	}
	if s.now().Sub(s.lastEmitAt) >= minEmitInterval {
		s.lastEmitAt = s.now()
		s.emitFrameLocked()
		return
	}
	s.armEmitTimerLocked(minEmitInterval - s.now().Sub(s.lastEmitAt))
}

func (s *Session) settleEmitLocked() {
	s.stopEmitTimerLocked()
	s.armEmitTimerLocked(resizeSettle)
}

func (s *Session) armEmitTimerLocked(
	delay time.Duration,
) {
	var t *time.Timer
	t = time.AfterFunc(delay, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.emitDueLocked(t)
	})
	s.emitTimer = t
}

func (s *Session) emitDueLocked(t *time.Timer) {
	if s.emitTimer != t {
		return
	}
	s.emitTimer = nil
	select {
	case <-s.done:
		return
	default:
	}
	s.lastEmitAt = s.now()
	s.emitFrameLocked()
}

func (s *Session) stopEmitTimerLocked() {
	if s.emitTimer != nil {
		s.emitTimer.Stop()
		s.emitTimer = nil
	}
}

func (s *Session) flushPendingEmitLocked() {
	if s.emitTimer == nil {
		return
	}
	s.stopEmitTimerLocked()
	s.lastEmitAt = s.now()
	s.emitFrameLocked()
}

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

func (s *Session) mutateModelLocked(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			s.resetModelLocked()
		}
	}()
	fn()
	s.screenGen++
}

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
