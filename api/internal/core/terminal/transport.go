package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
)

// Server→client wire format: PTY output is a BINARY message of one tag byte plus raw terminal
// bytes (the client decodes UTF-8 as a stream). FrameOutput appends; FrameSnapshot is a ground-state
// redraw applied onto a reset buffer. Control messages are TEXT JSON; the only one is exitMsg.
const (
	FrameOutput   byte = 0
	FrameSnapshot byte = 1
)

// exitMsg tells the client the process EXITED, sent once before the conn closes, so it can tell
// "shell ended" (close the tab) from a transport drop (reconnect).
type exitMsg struct {
	Type string `json:"type"` // always "exit"
	Code int    `json:"code"`
}

// inputMsg is the client→server frame for PTY input. A "theme" message carries the host's resolved
// Bg/Fg ("#rrggbb", the OSC 11/10 answers) and Dark, the polarity for the CSI ?997;n report.
type inputMsg struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
	Bg   string `json:"bg"`
	Fg   string `json:"fg"`
	Dark bool   `json:"dark"`
}

func (e *terminalEngine) Attach(
	ctx context.Context,
	sessionID string,
	conn WSConn,
) error {
	ent, ok := e.lookup(sessionID)
	if !ok {
		return fmt.Errorf("terminal: attach: %w: %s", ErrSessionNotFound, sessionID)
	}

	// Resolve the session to attach to and register the client in ONE lifecycle-lock
	// hold: a suspend (which requires no clients) cannot slip in between a restore and
	// this client counting.
	ent.mu.Lock()
	var fx effects
	switch ent.state {
	case stateSuspended:
		var err error
		if fx, err = e.restoreLocked(ctx, ent); err != nil {
			ent.mu.Unlock()
			fx.run()
			return fmt.Errorf("terminal: attach: %w", err)
		}
	case stateLive:
		// Already running: attach to it below.
	case stateExited, stateRemoved:
		ent.mu.Unlock()
		return fmt.Errorf("terminal: attach: %w: %s", ErrSessionNotFound, sessionID)
	}
	s := ent.sess.Load()
	ch, err := s.Attach()
	ent.mu.Unlock()
	fx.run()
	if err != nil {
		// The process exited and its reaper has not run yet. It is NOT restorable — tell
		// the client it exited rather than leaving it to reconnect into nothing.
		exitErr := e.writeExit(conn, s.ExitCode())
		closeErr := conn.Close()
		return errors.Join(fmt.Errorf("terminal: attach: %w", err), exitErr, closeErr)
	}

	writeDone := make(chan struct{})
	go e.writePump(conn, ch, func() (int, bool) { return e.exitStatus(ent, s) }, writeDone)
	e.readPump(conn, s)

	s.Detach(ch)
	<-writeDone

	e.persistOnDetach(ctx, ent, s)
	return nil
}

// exitStatus reports whether s's clients should be told it exited, and with what code:
// yes once its process is dead — unless the engine suspended it (Shutdown persisting it for
// the next boot), in which case it did not exit and the client should reconnect.
func (e *terminalEngine) exitStatus(ent *sessionEntry, s *session.Session) (int, bool) {
	select {
	case <-s.Done():
	default:
		return 0, false // dropped for overflow: the client reconnects to a fresh keyframe
	}
	ent.mu.Lock()
	suspended := ent.state == stateSuspended
	ent.mu.Unlock()
	return s.ExitCode(), !suspended
}

func (e *terminalEngine) writeExit(conn WSConn, code int) error {
	payload, err := json.Marshal(exitMsg{Type: "exit", Code: code})
	if err != nil {
		return fmt.Errorf("terminal: encode exit frame: %w", err)
	}
	return e.writeFrame(conn, websocket.TextMessage, payload)
}

// writeFrame sends one message under the write deadline (invariant B6).
func (e *terminalEngine) writeFrame(conn WSConn, messageType int, payload []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(e.cfg.writeWait)); err != nil {
		return fmt.Errorf("terminal: set write deadline: %w", err)
	}
	if err := conn.WriteMessage(messageType, payload); err != nil {
		return fmt.Errorf("terminal: write message: %w", err)
	}
	return nil
}

// warnUnlessClosed logs err, except that a connection already closed is not a failure.
func warnUnlessClosed(msg string, err error) {
	if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, os.ErrClosed) {
		slog.Warn(msg, "err", err)
	}
}

// maxCoalesceBytes caps the queued output writePump merges into one message: big enough to collapse
// bursts, small enough to bound per-message memory and renderer work.
const maxCoalesceBytes = 256 * 1024

// writePump forwards frames, then sends the exit frame and closes the conn; every write has a deadline
// (B6). Already-queued frames coalesce into one message without adding echo latency; a snapshot is
// always its own message. One reused buffer keeps steady-state output allocation-free.
func (e *terminalEngine) writePump(
	conn WSConn,
	ch <-chan session.OutputFrame,
	exitStatus func() (code int, exited bool),
	done chan<- struct{},
) {
	defer safego.Recover("terminal.writePump")
	defer func() {
		warnUnlessClosed("terminal: write pump: close conn", conn.Close())
		close(done)
	}()

	buf := make([]byte, 0, 64*1024)
	send := func(msg []byte) bool {
		err := e.writeFrame(conn, websocket.BinaryMessage, msg)
		warnUnlessClosed("terminal: write pump: client write failed", err)
		return err == nil
	}
	start := func(tag byte, data []byte) {
		buf = append(append(buf[:0], tag), data...)
	}

	var pending *session.OutputFrame // a snapshot that ended the previous drain
	recv := func() (session.OutputFrame, bool) {
		if pending == nil {
			f, ok := <-ch
			return f, ok
		}
		f := *pending
		pending = nil
		return f, true
	}
	for {
		frame, ok := recv()
		if !ok {
			break
		}
		if frame.Snapshot {
			start(FrameSnapshot, frame.Data)
			if !send(buf) {
				return
			}
			continue
		}
		start(FrameOutput, frame.Data)
		closed := false
	drain:
		for len(buf) < maxCoalesceBytes {
			select {
			case next, ok := <-ch:
				if !ok {
					closed = true
					break drain
				}
				if next.Snapshot {
					pending = &next
					break drain
				}
				buf = append(buf, next.Data...)
			default:
				break drain
			}
		}
		if !send(buf) {
			return
		}
		if closed {
			break
		}
	}
	if code, exited := exitStatus(); exited {
		warnUnlessClosed("terminal: write pump: exit frame not delivered", e.writeExit(conn, code))
	}
}

// readPump reads client messages from the WebSocket and dispatches them.
func (e *terminalEngine) readPump(
	conn WSConn,
	s *session.Session,
) {
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var msg inputMsg
		if jsonErr := json.Unmarshal(raw, &msg); jsonErr != nil {
			continue
		}

		switch msg.Type {
		case "resize":
			// Resize sends every client a keyframe at the new size. The model reflows as
			// the client's xterm does, so it matches what a live client already shows.
			if err := s.Resize(msg.Cols, msg.Rows); err != nil {
				slog.Warn("terminal: input: resize dropped", "session", s.ID(), "err", err)
			}
		case "theme":
			// Host theme changed: update the OSC 10/11 answers and push CSI ?997;n if DEC 2031 is on. A Crowbar
			// window has one theme, so record it for future births too.
			bg, fg := ParseHexColor(msg.Bg), ParseHexColor(msg.Fg)
			e.SetHostTheme(bg, fg)
			if err := s.SetTheme(bg, fg, msg.Dark); err != nil {
				slog.Warn("terminal: input: theme report dropped", "session", s.ID(), "err", err)
			}
		default:
			if err := s.Write([]byte(msg.Data)); err != nil {
				slog.Warn("terminal: input: keystrokes dropped", "session", s.ID(), "err", err)
			}
		}
	}
}
