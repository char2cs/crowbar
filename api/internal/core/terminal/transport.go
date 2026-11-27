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

const (
	FrameOutput   byte = 0
	FrameSnapshot byte = 1
)

type exitMsg struct {
	Type string `json:"type"`
	Code int    `json:"code"`
}

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
	case stateExited, stateRemoved:
		ent.mu.Unlock()
		return fmt.Errorf("terminal: attach: %w: %s", ErrSessionNotFound, sessionID)
	}
	s := ent.sess.Load()
	ch, err := s.Attach()
	ent.mu.Unlock()
	fx.run()
	if err != nil {
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

func (e *terminalEngine) exitStatus(ent *sessionEntry, s *session.Session) (int, bool) {
	select {
	case <-s.Done():
	default:
		return 0, false
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

func (e *terminalEngine) writeFrame(conn WSConn, messageType int, payload []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(e.cfg.writeWait)); err != nil {
		return fmt.Errorf("terminal: set write deadline: %w", err)
	}
	if err := conn.WriteMessage(messageType, payload); err != nil {
		return fmt.Errorf("terminal: write message: %w", err)
	}
	return nil
}

func warnUnlessClosed(msg string, err error) {
	if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, os.ErrClosed) {
		slog.Warn(msg, "err", err)
	}
}

const maxCoalesceBytes = 256 * 1024

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

	var pending *session.OutputFrame
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
			if err := s.Resize(msg.Cols, msg.Rows); err != nil {
				slog.Warn("terminal: input: resize dropped", "session", s.ID(), "err", err)
			}
		case "theme":
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
