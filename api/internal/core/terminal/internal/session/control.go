package session

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

const clientSendBuf = 256

// Attach registers a new client and returns its channel, pre-filled with ONE clean
// ground-state redraw serialized from the current model (§8.3/Appendix A). No raw replay,
// no DEC-mode preamble: the serialized state is self-contained, query-free, and fully
// terminated.
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
		// Sample the foreground-reset detector before serializing (§11.1 site #2) so a
		// re-attach inside the pumpStep debounce window of a SIGKILLed app never bakes its
		// stale alt/mouse modes into the new client.
		s.checkForegroundResetLocked()
		// Attach re-bases the emitter to the CURRENT model state, which would silently
		// drop any delta accumulated since the last emit for EXISTING clients (the attach
		// snapshot is not fanned out to them). Flush that pending delta to them first —
		// which also disarms a trailing frame-clock timer that would otherwise fire a
		// stale diff off the wrong base into this client — then serialize the snapshot
		// for the new client and re-prime. The serializer's output is self-contained,
		// query-free and fully terminated, so the snapshot ends exactly there.
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

// SetTheme propagates the host terminal's light/dark theme to the session (spec: theme
// propagation). It does two decoupled things, both required to make a foreground app's
// automatic theme follow a Crowbar theme switch:
//
//  1. Sets the model's OSC 10/11 QUERY-answer colours (bg/fg) so an app that detects its
//     theme by querying the background colour reads the truth — this alone fixes a freshly
//     started app and any later query. It never re-emits colour to the transparent client
//     xterm (see model.SetDefaultColors), so the glass background is untouched.
//
//  2. If the foreground app subscribed to theme-change notifications (DEC private mode 2031),
//     injects a CSI ?997;n report into the PTY so an ALREADY-running app re-queries and
//     switches live. The report is gated on the 2031 subscription (a shell that never opted
//     in must not receive it) and deduped by polarity so only the first push and each
//     light<->dark flip notify.
//
// The model mutation runs under s.mu (via the §8.5 recover backstop, like every model
// access); the PTY write is issued OFF the lock through s.Write, so it can never run the
// blocking ptmx.Write while the session lock is held (the C1 invariant, spec §3.8).
func (s *Session) SetTheme(
	bg color.Color,
	fg color.Color,
	dark bool,
) {
	s.mu.Lock()
	ta, ok := s.model.(model.ThemeAware)
	if s.model == nil || !ok {
		s.mu.Unlock()
		return
	}
	var notifyEnabled bool
	s.mutateModelLocked(func() {
		ta.SetDefaultColors(bg, fg)
		notifyEnabled = ta.ThemeNotifyEnabled()
	})
	// Emit on the first subscribed push and on every polarity flip; skip a redundant
	// same-polarity push. themeEmitted/themeEmittedDark only advance when we actually emit,
	// so a push while unsubscribed (notifyEnabled==false) still emits the first time the app
	// later subscribes and pushes.
	emit := notifyEnabled && (!s.themeEmitted || s.themeEmittedDark != dark)
	if emit {
		s.themeEmitted = true
		s.themeEmittedDark = dark
	}
	s.mu.Unlock()

	if emit {
		_ = s.Write(themeNotifySeq(dark))
	}
}

// themeNotifySeq returns the DEC mode 2031 theme-change report a terminal sends to a
// subscribed app: CSI ?997;1n for a dark theme, CSI ?997;2n for a light theme.
func themeNotifySeq(dark bool) []byte {
	if dark {
		return []byte("\x1b[?997;1n")
	}
	return []byte("\x1b[?997;2n")
}

// Resize updates the PTY window size and reshapes the model in lockstep under one s.mu
// hold, with the syscall before the model reshape and no intervening model.Write, so the
// model and PTY never disagree on the active width (§4.2/§8.3). The model reshape is
// panic-isolated (§8.5). Returns an error if the PTY is not live.
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
	s.cols, s.rows = int(cols), int(rows)
	s.mutateModelLocked(func() { s.model.Resize(int(cols), int(rows)) })
	// A resize can never be expressed as an absolute-addressed diff (the grid
	// dimensions themselves changed); force the next frame to be a full keyframe.
	s.emitter.Invalidate()
	s.dirty = true
	s.lastBlob = nil
	return nil
}

// Kill terminates the PTY process group. When the process already exited (ptmx == nil) it
// only calls shutdown(). Otherwise it closes the PTY, SIGKILLs the child's process group
// (so the children it spawned die with it, exactly as Terminate signals the group), nils
// ptmx, then calls shutdown() WITHOUT holding s.mu (shutdown re-acquires it inside once.Do).
// It returns only once the process has been reaped: Done is closed and ExitCode is final.
func (s *Session) Kill() {
	s.mu.Lock()
	if s.ptmx == nil {
		s.mu.Unlock()
		s.shutdown()
		return
	}
	_ = s.ptmx.Close()
	if s.cmd != nil && s.cmd.Process != nil {
		killSignal(s.cmd.Process)
	}
	s.ptmx = nil
	s.mu.Unlock()

	s.shutdown()
}

// Terminate gracefully quits the session's child process (spec §8): it sends
// a clean-exit SIGTERM via the terminateSignal seam — a PID-level action,
// never a PTY write, so the C-invariant (no keystrokes are ever synthesized
// to an app) holds — then waits up to grace for the process to exit on its
// own. A well-behaved CLI (e.g. Claude Code) gets the chance to flush state
// on a clean exit that an unconditional SIGKILL denies it. If the process is
// still alive once grace elapses (or there was no live process/signal to
// send), Terminate falls back to Kill's unconditional hard kill, so a caller
// never blocks longer than grace waiting for a wedged or signal-ignoring
// child.
//
// When the process already exited (ptmx == nil) there is nothing to signal,
// so shutdown() runs directly, exactly like Kill.
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

		// Stop the frame-clock timer (Task 7) under the same lock hold that
		// closes s.done, so a still-armed trailing emit can never fire after
		// teardown: its own s.done check would already guard against that,
		// but stopping it here also releases the timer goroutine promptly
		// instead of leaving it to wake up once more just to no-op.
		s.stopEmitTimerLocked()

		s.exitCode = code
		// The shell exiting on its own reaches teardown HERE, not through Kill: pump
		// reads EOF from the master and its deferred shutdown runs. Closing is this
		// path's job too — dropping the *os.File merely hands the descriptor to the
		// finalizer, and an idle daemon (allocating nothing, so never collecting) holds
		// it indefinitely; a later Kill cannot help, it takes the ptmx == nil branch.
		// A Kill racing a natural exit closed and nil'd ptmx under this same lock, so
		// the nil check is what keeps this from being a double close.
		if s.ptmx != nil {
			_ = s.ptmx.Close()
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

// SuspendEligible reports whether the engine may tear this session's PTY down to a
// suspended placeholder: never for a command session (a live agentic vendor CLI — restore
// could only exec the joined argv string as a bogus binary), never while a client is
// attached, and — unless force — only at an idle shell prompt. The engine calls it under
// its per-session lifecycle lock, which is also where Attach registers clients, so the
// answer cannot go stale before the suspend acts on it.
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
