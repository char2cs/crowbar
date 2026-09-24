package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/session"
)

// Server→client wire format.
//
// PTY output travels as BINARY WebSocket messages: one tag byte, then the bytes the
// client's terminal parses — no JSON string encoding, no escaping, no UTF-8 sanitizing,
// and no need to hold back a rune split across reads (the client's terminal decodes
// UTF-8 as a stream). The tag says how to apply the payload:
//
//	FrameOutput   incremental output — append it.
//	FrameSnapshot a self-contained ground-state redraw (attach, post-resize keyframe) —
//	              apply it onto a RESET buffer.
//
// Control messages travel as TEXT (JSON). The only one is exitMsg.
const (
	FrameOutput   byte = 0
	FrameSnapshot byte = 1
)

// exitMsg is the server→client frame that says the session's process EXITED — sent once,
// right before the engine closes the conn. It is what lets the client tell "the shell
// ended" (close the tab) from "the transport dropped" (reconnect): a conn that closes
// without it is a transport drop.
type exitMsg struct {
	Type string `json:"type"` // always "exit"
	Code int    `json:"code"`
}

// inputMsg is the client→server wire frame for PTY input.
//
// The "theme" type carries the host terminal's light/dark theme so a foreground app's
// automatic theme can follow a Crowbar theme switch: Bg/Fg are the resolved default
// background/foreground colours (as "#rrggbb", the values an OSC 11/10 query answers with)
// and Dark is the authoritative light/dark polarity for the CSI ?997;n theme-change report.
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
	default:
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
		e.writeExit(conn, s.ExitCode())
		_ = conn.Close()
		return fmt.Errorf("terminal: attach: %w", err)
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

func (e *terminalEngine) writeExit(conn WSConn, code int) {
	payload, err := json.Marshal(exitMsg{Type: "exit", Code: code})
	if err != nil {
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(e.cfg.writeWait))
	_ = conn.WriteMessage(websocket.TextMessage, payload)
}

// maxCoalesceBytes caps how much queued PTY output writePump merges into a
// single WebSocket message. Large enough to collapse bursts (build logs, cat,
// full-screen TUI redraws) into a handful of messages, small enough to bound
// per-message memory and keep the renderer's per-frame work reasonable.
const maxCoalesceBytes = 256 * 1024

// writePump forwards output frames from ch to the WebSocket, then — once ch closes
// because the process exited — sends the exit frame, and closes the conn so readPump
// unblocks. Every write carries a deadline, so a client that stops reading costs at most
// writeWait before the conn is torn down (invariant B6).
//
// Each iteration opportunistically drains frames ALREADY queued on ch — without blocking —
// and concatenates them into one message, so bursts coalesce while keystroke echo latency
// is unchanged. A snapshot is a coalescing barrier: it is always its own message, never
// merged with the output around it. One buffer is reused for every message (the conn
// copies it into the socket before WriteMessage returns), so steady-state output costs no
// allocation here.
func (e *terminalEngine) writePump(
	conn WSConn,
	ch <-chan session.OutputFrame,
	exitStatus func() (code int, exited bool),
	done chan<- struct{},
) {
	defer safego.Recover("terminal.writePump")
	defer func() {
		_ = conn.Close()
		close(done)
	}()

	buf := make([]byte, 0, 64*1024)
	send := func(msg []byte) bool {
		_ = conn.SetWriteDeadline(time.Now().Add(e.cfg.writeWait))
		return conn.WriteMessage(websocket.BinaryMessage, msg) == nil
	}
	start := func(tag byte, data []byte) {
		buf = append(append(buf[:0], tag), data...)
	}

	var pending *session.OutputFrame // a snapshot that ended the previous drain
	for {
		var frame session.OutputFrame
		if pending != nil {
			frame, pending = *pending, nil
		} else {
			f, ok := <-ch
			if !ok {
				break
			}
			frame = f
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
		e.writeExit(conn, code)
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
			// Resize also invalidates the diff base, so the frame the app's SIGWINCH
			// redraw produces is a keyframe: the client's reflowed buffer is replaced
			// by the model's, with no separate resync round-trip.
			_ = s.Resize(msg.Cols, msg.Rows)
		case "theme":
			// Host light/dark theme changed: update the model's OSC 10/11 query answers
			// and, if the foreground app subscribed to DEC 2031, push a live CSI ?997;n
			// report. Every session in a Crowbar window renders under ONE theme, so a
			// push at any of them is a statement about the host: record it for births too.
			bg, fg := ParseHexColor(msg.Bg), ParseHexColor(msg.Fg)
			e.SetHostTheme(bg, fg)
			s.SetTheme(bg, fg, msg.Dark)
		default:
			_ = s.Write([]byte(msg.Data))
		}
	}
}
