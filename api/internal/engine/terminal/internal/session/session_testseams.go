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
