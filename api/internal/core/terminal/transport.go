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

// outputMsg is the server→client output frame. Snapshot marks a self-contained
// ground-state redraw the client must apply onto a RESET buffer (attach redraw,
// post-resize keyframe) instead of appending like incremental output.
type outputMsg struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
	Snapshot  bool   `json:"snapshot,omitempty"`
}

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
	go e.writePump(conn, ent.id, ch, func() (int, bool) { return e.exitStatus(ent, s) }, writeDone)
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

// trailingIncompleteUTF8 returns the number of bytes at the end of b that form a
// truncated (not-yet-complete) multi-byte UTF-8 sequence — a rune whose lead byte
// has arrived but whose continuation bytes have not. Returns 0 when b ends on a
// rune boundary (empty, or ending in an ASCII byte or a complete sequence).
//
// writePump uses this to avoid splitting a multi-byte rune across two messages:
// the Data field is JSON-string-encoded, and json.Marshal replaces ANY invalid
// UTF-8 in a string with U+FFFD.
func trailingIncompleteUTF8(b []byte) int {
	// A truncated sequence is at most 3 bytes (a 4-byte rune missing up to 3).
	maxScan := 3
	if len(b) < maxScan {
		maxScan = len(b)
	}
	for i := 1; i <= maxScan; i++ {
		c := b[len(b)-i]
		if c < 0x80 {
			return 0 // ASCII byte: the tail is already on a rune boundary
		}
		if c >= 0xC0 { // a lead byte
			need := 2
			switch {
			case c >= 0xF0:
				need = 4
			case c >= 0xE0:
				need = 3
			}
			if i < need {
				return i // lead byte present but continuation bytes missing
			}
			return 0 // full sequence present
		}
		// 0x80..0xBF: continuation byte — keep scanning back for the lead byte
	}
	return 0 // no lead byte within the last 3 bytes (malformed) — leave as-is
}

// writePump forwards output frames from ch to the WebSocket, then — once ch closes
// because the process exited — sends the exit frame, and closes the conn so readPump
// unblocks. Every write carries a deadline, so a client that stops reading costs at most
// writeWait before the conn is torn down (invariant B6).
//
// Each iteration opportunistically drains frames ALREADY queued on ch — without blocking —
// and concatenates them into one message, so bursts coalesce while keystroke echo latency
// is unchanged. A trailing incomplete UTF-8 rune is held back (via pending) and prepended
// to the next message so json.Marshal never corrupts a split multi-byte glyph.
//
//nolint:gocyclo // cohesive coalescing/UTF-8-holdback state machine; splitting it would obscure the single-drain-loop invariant.
func (e *terminalEngine) writePump(
	conn WSConn,
	sessionID string,
	ch <-chan session.OutputFrame,
	exitStatus func() (code int, exited bool),
	done chan<- struct{},
) {
	defer safego.Recover("terminal.writePump")
	defer func() {
		_ = conn.Close()
		close(done)
	}()

	// writeMsg marshals and sends one wire frame; false means the socket died.
	writeMsg := func(data []byte, snapshot bool) bool {
		payload, err := json.Marshal(outputMsg{SessionID: sessionID, Data: string(data), Snapshot: snapshot})
		if err != nil {
			return true
		}
		_ = conn.SetWriteDeadline(time.Now().Add(e.cfg.writeWait))
		return conn.WriteMessage(websocket.TextMessage, payload) == nil
	}

	var pending []byte
	for frame := range ch {
		// Snapshot frames are coalescing BARRIERS: a snapshot is a self-contained redraw
		// the client applies onto a reset buffer, so it must never be merged into (or
		// split across) incremental output. A held-back partial rune belongs to the
		// pre-snapshot stream the reset supersedes — drop it.
		if frame.Snapshot {
			pending = pending[:0]
			if !writeMsg(frame.Data, true) {
				return
			}
			continue
		}

		buf := make([]byte, 0, len(pending)+len(frame.Data))
		buf = append(buf, pending...)
		buf = append(buf, frame.Data...)
		pending = pending[:0]
		closed := false
		var snapshotAfter *session.OutputFrame

	drain:
		for len(buf) < maxCoalesceBytes {
			select {
			case next, ok := <-ch:
				if !ok {
					closed = true
					break drain
				}
				if next.Snapshot {
					snap := next
					snapshotAfter = &snap
					break drain
				}
				buf = append(buf, next.Data...)
			default:
				break drain
			}
		}

		if !closed && snapshotAfter == nil {
			if n := trailingIncompleteUTF8(buf); n > 0 {
				pending = append(pending, buf[len(buf)-n:]...)
				buf = buf[:len(buf)-n]
			}
		}

		if len(buf) > 0 && !writeMsg(buf, false) {
			return
		}
		if snapshotAfter != nil && !writeMsg(snapshotAfter.Data, true) {
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
			_ = s.Resize(msg.Cols, msg.Rows)
		case "resync":
			// Post-resize convergence: re-emit the model snapshot to attached clients
			// (no-op at an idle shell prompt — see Session.Resync).
			_ = s.Resync()
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
