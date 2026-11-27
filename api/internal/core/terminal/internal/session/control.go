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
		s.checkForegroundResetLocked()
		s.flushPendingEmitLocked()
		if redraw := s.serializeLocked(); len(redraw) > 0 {
			cl.send <- OutputFrame{SessionID: s.id, Data: redraw, Snapshot: true}
		}
		s.primeLocked()
	}

	s.clients[cl] = struct{}{}
	return cl.send, nil
}

func (s *Session) Detach(
	ch <-chan OutputFrame,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.detachLocked(ch)
}

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

func themeNotifySeq(dark bool) []byte {
	if dark {
		return []byte("\x1b[?997;1n")
	}
	return []byte("\x1b[?997;2n")
}

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
	s.emitter.Invalidate()
	s.dirty = true
	s.lastBlob = nil
	if len(s.clients) > 0 {
		s.settleEmitLocked()
	}
	return nil
}

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

func (s *Session) killChild() {
	if s.cmd == nil || s.cmd.Process == nil {
		return
	}
	if err := killSignal(s.cmd.Process); err != nil {
		slog.Warn("terminal: kill: child not signalled", "session", s.id, "err", err)
	}
}

func closePTY(ptmx *os.File) {
	if err := ptmx.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		slog.Warn("terminal: close pty", "err", err)
	}
}

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
		s.Kill()
		return
	}

	select {
	case <-done:
		return
	case <-time.After(grace):
	}

	s.Kill()
}

func (s *Session) shutdown() {
	s.once.Do(func() {
		code := reap(s.cmd)

		s.mu.Lock()
		defer s.mu.Unlock()

		s.stopEmitTimerLocked()

		s.exitCode = code
		if s.ptmx != nil {
			closePTY(s.ptmx)
		}
		s.ptmx = nil
		if s.model != nil {
			s.model.Close()
		}
		close(s.done)
		for cl := range s.clients {
			close(cl.send)
		}
		s.clients = make(map[*client]struct{})
	})
}

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
