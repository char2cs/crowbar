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

// ScreenText renders the session's VISIBLE screen as plain text, and reports whether
// it has moved since generation `since`.
//
// The (value, changed) shape mirrors Snapshot's, and for the same reason: the caller
// polls, most polls find nothing new, and the cheap answer must not cost a render.
// An unchanged screen returns no text at all — the caller already has it — so a chat
// parked on a modal costs one integer compare per poll no matter how long it sits
// there.
//
// A placeholder (suspended, model == nil) has no screen: it reports gen 0, unchanged.
// So does a model whose backend does not implement ScreenReader, which is the same
// guarded-optional-interface treatment ThemeAware and ModelHealth get.
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

// InjectLocal feeds a clean-ANSI chunk into THIS session's model only — never the live wire
// and never the persisted .buf. It is the sole sanctioned way for the engine to push a
// synthetic, daemon-authored on-screen notice (restore/suspend) so it surfaces on the next
// Serialize (§12).
func (s *Session) InjectLocal(
	b []byte,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.injectLocalLocked(b)
}

// injectLocalLocked is the lock-free core of InjectLocal. Caller holds s.mu. It feeds bytes
// through writeModelLocked (the §8.5 recover) so a parse panic on a daemon notice cannot
// escape, and defensively drives the model to the primary buffer first so a notice can never
// land in a transient alt buffer (§12). It marks the session dirty and drops the cache; it
// never fans out.
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

// ForceSuspendSnapshot performs the §11.2 teardown and the suspending serialize as ONE
// uninterrupted s.mu critical section, so no live pumpStep chunk can interleave between the
// teardown and the serialize and repaint alt content onto the model just forced to primary.
// It drives the model to the primary buffer (OnForegroundReset), injects the notice into
// that primary screen, serializes a clean primary blob, caches it, and returns it for the
// engine to persist verbatim WITHOUT re-Snapshotting. Caller must NOT already hold s.mu.
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

// Health reports the session's parse-health for the engine's Stats observability surface
// (§9.4): whether the model is in the sticky degraded state, and the total recovered parse
// panics. The count combines the model's own ModelHealth.ParsePanics() (panics x/vt's Write
// recovered internally, self-healing without forcing the raw fallback) with the session-
// level s.modelPanics backstop counter (panics that escaped a model method —
// Resize/Serialize/Emit/Prime/teardown — into the §8.5 recover and DID flip the session to
// raw), so neither is write-only and a blanked-and-reparsed session is observable. A
// placeholder (model ==
// nil) or a backend that does not implement ModelHealth contributes only s.modelPanics.
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
		// Stable-from-spawn estimate derived from the model's dims (see
		// model.EmitterGridBytes): counting the grid only after the first
		// Prime would make the session's reported size jump ~3× at its
		// first output, destabilizing the maintenance ceiling arithmetic.
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
