package session

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

// SetNowForTest pins THIS SESSION's frame clock (s.now).
//
// It exists because the coalescing decision in scheduleEmitLocked is a comparison against the
// wall clock, so "these N chunks all landed inside ONE interval" is not something a test can
// establish by running fast — it is something it can only establish by CONTROLLING the clock.
// Without it the burst test had to race its own setup against an 8 ms window, and lost under
// parallel -race load.
//
// Deliberately per-session and taken under s.mu, NOT a package-level var: the trailing emit
// timer reads the clock from its own goroutine, so a global would be an unsynchronised
// cross-goroutine write — which is exactly the data race the first cut of this seam produced.
// Production never calls this.
func (s *Session) SetNowForTest(fn func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = fn
}

// PumpNotifyForTest returns the session's pump-progress signal: a 1-buffered, coalescing
// channel that pumpStep publishes to — last in its critical section — every time it has
// FULLY processed a chunk of PTY output (model written, frame emitted, s.dirty set).
//
// It is the real signal that replaces "sleep and hope the shell has spoken by now". A PTY
// is an asynchronous source: a duration is a guess about how fast a fork/exec/prompt is,
// and under CI load the guess is wrong. Waiters block on this edge and re-read the
// observable they actually care about (the screen, IsIdle, dirty), so a slow machine makes
// them slower, never wrong.
//
// Coalescing means a wakeup proves "at least one chunk landed since you last drained it",
// NOT a 1:1 chunk count — so always re-check a real predicate after waking rather than
// counting wakeups. Production never receives from this channel.
func (s *Session) PumpNotifyForTest() <-chan struct{} { return s.pumpNotify }

// SerializedForTest returns the session's current screen as serializer redraw bytes. It is
// deliberately NON-CONSUMING: unlike Snapshot it does not clear the dirty bit, so a test may
// use it as a readiness predicate without perturbing a later cadence flush (the same
// contract).
func (s *Session) SerializedForTest() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.serializeLocked()
}

// SetNewModelForTest overrides the package-level model-construction seam and returns a
// restore function. It exists so an OUT-OF-PACKAGE test (the engine's Stats degraded-count
// test) can spawn a real PTY session backed by a fake model that reports a degraded
// parse-health surface — a state no deterministic PTY input can produce because the real vt
// emulator never panics on test bytes. Production never calls this; the in-package tests use
// the private newModel var directly.
func SetNewModelForTest(
	fn func(cols, rows, sb int) (model.TerminalModel, model.Serializer),
) (restore func()) {
	orig := newModel
	newModel = fn
	return func() { newModel = orig }
}
