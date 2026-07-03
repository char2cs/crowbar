package session

import (
	"os"

	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/model"
)

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

// NewDoneClosedForTest returns a Session that reports IsLive()==true (it holds a non-nil
// *os.File) yet whose done channel is already closed, so a subsequent Attach() returns an
// error. It exists so an OUT-OF-PACKAGE test can drive the engine's Attach error-path guard
// (a live session whose Attach fails) — a TOCTOU state that production reaches only when the
// session dies in the window between the engine's IsLive() check and its Attach() call.
// Production never calls this.
func NewDoneClosedForTest(
	id string,
	shell string,
	cwd string,
	profileID string,
) *Session {
	s := newBareSession(id, shell, cwd, profileID)
	// A closed *os.File is non-nil, so IsLive() (ptmx != nil) reports true.
	f, _ := os.Open(os.DevNull)
	if f != nil {
		_ = f.Close()
		s.ptmx = f
	}
	// Close done THROUGH the once guard so a later shutdown() (e.g. via Kill during engine
	// Shutdown) is a no-op rather than a double-close panic.
	s.once.Do(func() { close(s.done) })
	return s
}

// NewModelDriven is the test-facing constructor for a model-driven session. It mirrors New's
// body exactly, adding ModelDriven: true to spawnParams so pumpStep takes the model-derived
// fan-out path (§3.1) instead of raw streaming.
func NewModelDriven(
	id string,
	shell string,
	cwd string,
	profileID string,
	env []string,
	cols int,
	rows int,
	scrollback int,
) (*Session, error) {
	s := newBareSession(id, shell, cwd, profileID)
	if err := s.spawn(env, spawnParams{Cols: cols, Rows: rows, ScrollbackLines: scrollback, ModelDriven: true}); err != nil {
		return nil, err
	}
	return s, nil
}

// forceModelPanicForTest drives a model-driven session into the degraded/raw-fallback state
// the same way a real recovered Write/Resize/Serialize/Emit panic would (via the §8.5
// modelPanics counter), without needing adversarial PTY bytes. It exists so an
// out-of-package-adjacent test can exercise useModelDrivenLocked's fallback gate
// deterministically. Production never calls this.
func forceModelPanicForTest(s *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelPanics++
}

// forceEmitPanicForTest installs an emitForTest hook that panics on the very next
// emitLocked call, then clears itself so only that one call panics. It exists so a test
// can make the EMIT path (not writeModelLocked) degrade — the boundary the production
// pumpStep fallback (fan the triggering chunk's raw bytes out rather than dropping them)
// exists for. No adversarial PTY input can panic the real emitter deterministically, so
// this seam is the only way to reach that boundary. Production never calls this.
func forceEmitPanicForTest(s *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emitForTest = func(m model.TerminalModel) ([]byte, bool) {
		s.emitForTest = nil
		panic("forceEmitPanicForTest: simulated emit panic")
	}
}
