package session

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

// Snapshot returns the session's persisted blob and whether it changed since the last
// flush (§8.4). It samples the foreground reset, reuses lastBlob when clean, otherwise
// serializes header+redraw under one s.mu hold and clears the dirty bit in that same hold.
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

// ScreenText renders the VISIBLE screen as text if it moved since generation since; unchanged
// returns no text, so polling costs one compare. A placeholder or a backend without
// ScreenReader reports gen 0, unchanged.
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

// header builds the mandatory CRWB1 size line, sourced entirely from the model's
// HeaderState so it never re-parses the stream (§12). Caller holds s.mu; only live sessions
// (model != nil) call it.
func (s *Session) header() string {
	cols, rows, alt, sb := s.model.HeaderState()
	altBit := 0
	if alt {
		altBit = 1
	}
	return fmt.Sprintf("CRWB1 %d %d %d %d\n", cols, rows, altBit, sb)
}

// InjectLocal feeds clean ANSI into this session's model only (never the wire or the .buf):
// the sanctioned way to show a daemon-authored notice on the next Serialize.
func (s *Session) InjectLocal(
	b []byte,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.injectLocalLocked(b)
}

// injectLocalLocked feeds bytes through writeModelLocked's recover, on the primary buffer so a
// notice never lands in an alt screen; it marks dirty and never fans out. Caller holds s.mu.
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

// ForceSuspendSnapshot forces primary, injects the notice, serializes and caches in ONE s.mu
// section, so no pump chunk can repaint alt content before the serialize. Caller must NOT hold s.mu.
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

// Health reports degraded state and recovered parse panics: the model's own ModelHealth count
// plus s.modelPanics (panics that escaped into the session recover and flipped it to raw).
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

// ModelBytes returns the session's estimated resident size for the engine's memory ceiling:
// the model's grid+scrollback estimate plus the cached blob (§9.4) and the diff emitter's
// retained lastGrid estimate.
func (s *Session) ModelBytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := s.model.ModelBytes() + int64(len(s.lastBlob))
	if s.emitter != nil {
		// Estimated from the model's dims (model.EmitterGridBytes) so the reported size is stable
		// from spawn for the maintenance ceiling.
		cols, rows, _, _ := s.model.HeaderState()
		total += model.EmitterGridBytes(cols, rows)
	}
	return total
}

// DropCachedBlob reclaims the blob cache under memory pressure (§9.4 Phase-3 pre-step): it
// nils lastBlob and marks the session dirty so the next Snapshot re-serializes a correct,
// current blob. Returns the bytes reclaimed.
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
