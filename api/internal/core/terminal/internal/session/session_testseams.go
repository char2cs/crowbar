package session

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

// SetNowForTest pins this session's frame clock, so a test can prove a burst landed inside ONE coalescing
// interval instead of racing an 8 ms window. Per-session under s.mu, not a package var, because the
// trailing emit timer reads the clock from its own goroutine.
func (s *Session) SetNowForTest(fn func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = fn
}

// PumpNotifyForTest returns the 1-buffered, coalescing channel pumpStep signals once a chunk is fully
// processed, so tests block on real progress instead of sleeping. A wakeup means at least one chunk, not
// one per chunk: always re-check a real predicate.
func (s *Session) PumpNotifyForTest() <-chan struct{} { return s.pumpNotify }

// SerializedForTest returns the current screen as redraw bytes without clearing the dirty bit, so a test
// can poll readiness without perturbing a later flush.
func (s *Session) SerializedForTest() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.serializeLocked()
}

// SetNewModelForTest overrides the model-construction seam and returns a restore, so an out-of-package
// test can back a real PTY session with a fake degraded model the real emulator cannot produce.
func SetNewModelForTest(
	fn func(cols, rows, sb int) (model.TerminalModel, model.Serializer),
) (restore func()) {
	orig := newModel
	newModel = fn
	return func() { newModel = orig }
}
