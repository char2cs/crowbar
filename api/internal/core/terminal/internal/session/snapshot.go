package session

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

func (s *Session) Snapshot() (blob []byte, changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkForegroundResetLocked()
	if !s.dirty && s.lastBlob != nil {
		return s.lastBlob, false
	}
	blob = append([]byte(s.header()), s.serializeLocked()...)
	s.lastBlob = blob
	s.dirty = false
	return blob, true
}

func (s *Session) ScreenText(since uint64) (text string, gen uint64, changed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model == nil {
		return "", 0, false
	}
	if s.screenGen == since {
		return "", s.screenGen, false
	}
	reader, ok := s.model.(model.ScreenReader)
	if !ok {
		return "", s.screenGen, false
	}
	return reader.ScreenText(), s.screenGen, true
}

func (s *Session) ScreenState() (st model.ScreenState, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model == nil {
		return model.ScreenState{}, false
	}
	reader, ok := s.model.(model.ScreenStateReader)
	if !ok {
		return model.ScreenState{}, false
	}
	return reader.ScreenState(), true
}

func (s *Session) header() string {
	cols, rows, alt, sb := s.model.HeaderState()
	altBit := 0
	if alt {
		altBit = 1
	}
	return fmt.Sprintf("CRWB1 %d %d %d %d\n", cols, rows, altBit, sb)
}

func (s *Session) InjectLocal(
	b []byte,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.injectLocalLocked(b)
}

func (s *Session) injectLocalLocked(
	b []byte,
) {
	if s.model == nil {
		return
	}
	if _, _, alt, _ := s.model.HeaderState(); alt {
		s.writeModelLocked([]byte("\x1b[?1049l\x1b[?47l\x1b[?1047l"))
	}
	s.writeModelLocked(b)
	s.dirty = true
	s.lastBlob = nil
}

func (s *Session) ForceSuspendSnapshot(
	notice []byte,
) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mutateModelLocked(s.model.OnForegroundReset)
	s.injectLocalLocked(notice)
	blob := append([]byte(s.header()), s.serializeLocked()...)
	s.dirty = false
	s.lastBlob = blob
	return blob
}

func (s *Session) Health() (degraded bool, parsePanics int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	parsePanics = s.modelPanics
	if h, ok := s.model.(model.ModelHealth); ok {
		if h.Degraded() {
			degraded = true
		}
		parsePanics += h.ParsePanics()
	}
	return degraded, parsePanics
}

func (s *Session) ModelBytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := s.model.ModelBytes() + int64(len(s.lastBlob))
	if s.emitter != nil {
		cols, rows, _, _ := s.model.HeaderState()
		total += model.EmitterGridBytes(cols, rows)
	}
	return total
}

func (s *Session) DropCachedBlob() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastBlob == nil {
		return 0
	}
	n := int64(len(s.lastBlob))
	s.lastBlob = nil
	s.dirty = true
	return n
}
