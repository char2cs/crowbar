package ws

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/origin"
)

const (
	pingInterval = 30 * time.Second
	pongTimeout  = 60 * time.Second
	writeTimeout = 10 * time.Second
	sendBuffer   = 64
)

// upgrader rejects cross-origin WebSocket upgrades from a non-allow-listed Origin
// so a malicious website can't open the per-workspace event stream through the
// user's browser. A blanket "return true" let any page hijack the socket; see
// origin.Allowed for the allowlist (Tauri webview, loopback, env overrides).
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		o := r.Header.Get("Origin")
		if origin.Allowed(o, r.Host) {
			return true
		}
		slog.WarnContext(r.Context(), "ws: rejected cross-origin upgrade",
			"origin", o, "host", r.Host)
		return false
	},
}

type client struct {
	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once
}

func newClient() *client {
	return &client{
		send: make(chan []byte, sendBuffer),
		done: make(chan struct{}),
	}
}

// closeDone signals the client's writePump to stop, exactly once. Both the
// normal removal path and a slow-consumer overflow disconnect route through it,
// so done is never double-closed (which would panic).
func (c *client) closeDone() {
	c.closeOnce.Do(func() { close(c.done) })
}

func readPump(
	conn *websocket.Conn,
) {
	if DefaultKeepAlive.Arm(conn) != nil {
		return
	}
	for {
		if _, _, err := conn.NextReader(); err != nil {
			return
		}
	}
}

// writePump writes the snapshot frames first, in order and without dropping, then
// enters the live select loop draining cl.send and emitting pings. Flushing the
// snapshot ahead of the loop preserves the snapshot-before-live wire ordering and
// guarantees the snapshot is never truncated by the bounded cl.send buffer.
func writePump(
	conn *websocket.Conn,
	cl *client,
	snapshot [][]byte,
) {
	ticker := DefaultKeepAlive.ticker()
	defer func() {
		ticker.Stop()
		libs.CloseLogged(context.Background(), "ws: write pump: close", conn)
	}()
	if !flushSnapshot(conn, cl, snapshot) {
		return
	}
	for {
		if !writeNext(conn, cl, ticker) {
			return
		}
	}
}

// flushSnapshot writes every snapshot frame to the conn in order, blocking on
// each write. It returns false if a write fails or the client is torn down, so
// writePump can abort before entering the live loop.
func flushSnapshot(
	conn *websocket.Conn,
	cl *client,
	snapshot [][]byte,
) bool {
	for _, msg := range snapshot {
		select {
		case <-cl.done:
			return false
		default:
		}
		if !writeMessage(conn, websocket.TextMessage, msg) {
			return false
		}
	}
	return true
}

func writeNext(
	conn *websocket.Conn,
	cl *client,
	ticker *time.Ticker,
) bool {
	select {
	case msg := <-cl.send:
		return writeFrame(conn, msg)
	case <-ticker.C:
		return writeMessage(conn, websocket.PingMessage, nil)
	case <-cl.done:
		return false
	}
}

func writeFrame(conn *websocket.Conn, msg []byte) bool {
	return writeMessage(conn, websocket.TextMessage, msg)
}

// writeMessage writes one message under the write timeout; a socket that
// cannot take the deadline cannot take the write either.
func writeMessage(conn *websocket.Conn, kind int, msg []byte) bool {
	if conn.SetWriteDeadline(time.Now().Add(writeTimeout)) != nil {
		return false
	}
	return conn.WriteMessage(kind, msg) == nil
}
