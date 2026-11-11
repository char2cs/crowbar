package session

import (
	"errors"
	"fmt"
	"image/color"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

const clientSendBuf = 256

// Attach registers a client and returns its channel pre-filled with ONE ground-state
// redraw of the current model: self-contained, query-free, no raw replay.
func (s *Session) Attach() (<-chan OutputFrame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.done:
		return nil, fmt.Errorf("session: attach: session %s is dead", s.id)
	default:
	}

	cl := &client{send: make(chan OutputFrame, clientSendBuf)}

	if s.model != nil {
		// Sample the foreground-reset detector before serializing so a
		// re-attach inside the pumpStep debounce window of a SIGKILLed app never bakes its
		// stale alt/mouse modes into the new client.
		s.checkForegroundResetLocked()
		// Re-basing the emitter would drop the delta pending for EXISTING clients, so flush it to them
		// first (which also disarms a trailing timer that would diff off the wrong base), then snapshot.
		s.flushPendingEmitLocked()
		if redraw := s.serializeLocked(); len(redraw) > 0 {
			cl.send <- OutputFrame{SessionID: s.id, Data: redraw, Snapshot: true}
		}
		s.primeLocked()
	}

	s.clients[cl] = struct{}{}
	return cl.send, nil
}

// Detach removes a client from the fan-out set and closes its channel.
func (s *Session) Detach(
	ch <-chan OutputFrame,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.detachLocked(ch)
}

// detachLocked removes the client whose send channel matches ch. Caller must hold s.mu.
func (s *Session) detachLocked(
	ch <-chan OutputFrame,
) {
	for cl := range s.clients {
		if ch == (<-chan OutputFrame)(cl.send) {
			delete(s.clients, cl)
			close(cl.send)
			return
		}
	}
}

// Write sends data to the PTY stdin. Returns an error if the PTY is not live.
func (s *Session) Write(
	data []byte,
) error {
	s.mu.Lock()
	ptmx := s.ptmx
	s.mu.Unlock()
	if ptmx == nil {
		return fmt.Errorf("session: write: session not live")
	}
	_, err := ptmx.Write(data)
	if err != nil {
		return fmt.Errorf("session: write: %w", err)
	}
	return nil
}

// SetTheme sets the model's OSC 10/11 query-answer colours (never re-emitted to the client), and
// if the app subscribed to mode 2031 sends a polarity-deduped CSI ?997;n so a running app switches.
// The PTY write runs off s.mu so ptmx.Write never blocks under the lock; its error is returned.
func (s *Session) SetTheme(
	bg color.Color,
	fg color.Color,
	dark bool,
) error {
	s.mu.Lock()
	ta, ok := s.model.(model.ThemeAware)
	if s.model == nil || !ok {
		s.mu.Unlock()
		return nil
	}
	var notifyEnabled bool
	s.mutateModelLocked(func() {
		ta.SetDefaultColors(bg, fg)
		notifyEnabled = ta.ThemeNotifyEnabled()
	})
	// Emit on the first subscribed push and each polarity flip; the emitted state only advances
	// on an actual emit, so a push while unsubscribed still emits once the app subscribes.
	emit := notifyEnabled && (!s.themeEmitted || s.themeEmittedDark != dark)
	if emit {
		s.themeEmitted = true
		s.themeEmittedDark = dark
	}
	s.mu.Unlock()

	if emit {
		return s.Write(themeNotifySeq(dark))
	}
	return nil
}

// themeNotifySeq returns the DEC mode 2031 theme-change report a terminal sends to a
// subscribed app: CSI ?997;1n for a dark theme, CSI ?997;2n for a light theme.
func themeNotifySeq(dark bool) []byte {
	if dark {
		return []byte("\x1b[?997;1n")
	}
	return []byte("\x1b[?997;2n")
}

// Resize reshapes the PTY then the model under one s.mu hold with no write between, so they
// never disagree on width; the reshape is panic-isolated. Errors if the PTY is not live.
func (s *Session) Resize(
	cols uint16,
	rows uint16,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptmx == nil {
		return fmt.Errorf("session: resize: session not live")
	}
	if err := pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows}); err != nil {
		return fmt.Errorf("session: resize: %w", err)
	}
	if int(cols) == s.cols && int(rows) == s.rows {
		return nil
	}
	s.cols, s.rows = int(cols), int(rows)
	s.mutateModelLocked(func() { s.model.Resize(int(cols), int(rows)) })
	// A resize can never be expressed as an absolute-addressed diff. The frame clock sends the
	// keyframe once the resizes settle, since an app idle on SIGWINCH prints nothing; with no
	// client, the next attach primes the invalidated emitter.
	s.emitter.Invalidate()
	s.dirty = true
	s.lastBlob = nil
	if len(s.clients) > 0 {
		s.settleEmitLocked()
	}
	return nil
}

// Kill closes the PTY and SIGKILLs the child's process group so its children die too, then
// runs shutdown() without s.mu (it re-acquires it). It returns once the process is reaped.
func (s *Session) Kill() {
	s.mu.Lock()
	if s.ptmx == nil {
		s.mu.Unlock()
		s.shutdown()
		return
	}
	closePTY(s.ptmx)
	s.killChild()
	s.ptmx = nil
	s.mu.Unlock()

	s.shutdown()
}

// killChild SIGKILLs the child's process group, if the session has a child.
func (s *Session) killChild() {
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}
	if err := killSignal(s.cmd.Process); err != nil {
		slog.Warn("terminal: kill: child not signalled", "session", s.id, "err", err)
	}
}

// closePTY closes the PTY master. Kill and shutdown both nil ptmx under s.mu, so a second
// close should not reach it; os.ErrClosed is filtered as a defence, not as an expected case.
func closePTY(ptmx *os.File) {
	if err := ptmx.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		slog.Warn("terminal: close pty", "err", err)
	}
}

// Terminate SIGTERMs the child (a PID-level action, never a synthesized keystroke) so it can
// flush state, waits up to grace, then falls back to Kill; a caller never blocks past grace.
// With no live process it runs shutdown() directly, like Kill.
func (s *Session) Terminate(grace time.Duration) {
	s.mu.Lock()
	if s.ptmx == nil {
		s.mu.Unlock()
		s.shutdown()
		return
	}
	var proc *os.Process
	if s.cmd != nil {
		proc = s.cmd.Process
	}
	done := s.done
	s.mu.Unlock()

	if proc == nil {
		s.Kill()
		return
	}

	if err := terminateSignal(proc); err != nil {
		// Already gone, or signalling failed outright: the hard-kill fallback
		// is safe either way (Kill/shutdown are idempotent via s.once).
		s.Kill()
		return
	}

	select {
	case <-done:
		// Exited on its own within the grace window: shutdown() already ran
		// (via pump()'s exit path), nothing left to do.
		return
	case <-time.After(grace):
	}

	// Still alive after the grace window: fall back to the unconditional
	// hard kill.
	s.Kill()
}

// shutdown reaps the child process, tears down the model, and closes the done + client
// channels exactly once. The cmd.Wait() here is the ONLY reap. once guards against a double
// Wait when both Kill() and pump()'s exit reach shutdown.
func (s *Session) shutdown() {
	s.once.Do(func() {
		code := reap(s.cmd)

		s.mu.Lock()
		defer s.mu.Unlock()

		// Stop the frame-clock timer under the lock that closes s.done, so a trailing emit never
		// fires after teardown and the timer goroutine is released promptly.
		s.stopEmitTimerLocked()

		s.exitCode = code
		// A natural shell exit reaches teardown here, so this path must close the master: the finalizer
		// may never run on an idle daemon. A racing Kill nils ptmx under this lock, preventing a double close.
		if s.ptmx != nil {
			closePTY(s.ptmx)
		}
		s.ptmx = nil
		if s.model != nil {
			s.model.Close()
		}
		// Done first: a client that sees its channel close asks Done whether we exited.
		close(s.done)
		for cl := range s.clients {
			close(cl.send)
		}
		s.clients = make(map[*client]struct{})
	})
}

// SuspendEligible: never for a command session (restore cannot re-exec it) or with a client
// attached, and unless force only at an idle prompt. Called under the engine's lifecycle lock,
// where Attach also registers, so the answer cannot go stale.
func (s *Session) SuspendEligible(force bool) bool {
	if s.command {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.clients) > 0 {
		return false
	}
	return force || s.isIdleLocked()
}

// reap waits for cmd (the session's single reap) and returns its exit code: 0 on a clean
// exit, the status of an ExitError, or -1 when there is no process or the code is unknown.
func reap(cmd *exec.Cmd) int {
	if cmd == nil {
		return -1
	}
	err := cmd.Wait()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
