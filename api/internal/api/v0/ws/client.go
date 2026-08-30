package ws

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

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

	// coalesceMu guards pending — see coalesce's own doc comment.
	coalesceMu sync.Mutex
	pending    map[string][]byte
	// wake carries no data: it only tells writeNext "something in pending is
	// worth checking now". Buffered 1 so a burst of coalesce calls between
	// two writeNext iterations collapses to a single wakeup, exactly the way
	// the payloads themselves already collapse to one pending value per key.
	wake chan struct{}
}

func newClient() *client {
	return &client{
		send:    make(chan []byte, sendBuffer),
		done:    make(chan struct{}),
		pending: make(map[string][]byte),
		wake:    make(chan struct{}, 1),
	}
}

// coalesce stores data as the latest still-undelivered payload for key,
// superseding whatever this client had pending for that SAME key, and wakes
// the write loop. Never blocks and never disconnects the client — the
// opposite of the ordinary send path's overflow policy, and deliberately so:
// see StreamDef.CoalesceKey's own doc comment for which streams this is
// correct for.
func (c *client) coalesce(key string, data []byte) {
	c.coalesceMu.Lock()
	c.pending[key] = data
	c.coalesceMu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// drainPending removes and returns every currently pending coalesced
// payload. Iteration order across DIFFERENT keys is unspecified — callers
// only rely on coalesce for streams whose own values carry no cross-key
// ordering requirement (see StreamDef.CoalesceKey).
func (c *client) drainPending() [][]byte {
	c.coalesceMu.Lock()
	defer c.coalesceMu.Unlock()
	if len(c.pending) == 0 {
		return nil
	}
	out := make([][]byte, 0, len(c.pending))
	for key, data := range c.pending {
		out = append(out, data)
		delete(c.pending, key)
	}
	return out
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
	_ = conn.SetReadDeadline(time.Now().Add(pongTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongTimeout))
	})
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
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
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
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if conn.WriteMessage(websocket.TextMessage, msg) != nil {
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
	// Ordered traffic drains first, ALWAYS, whenever any is already waiting —
	// a plain, non-blocking check before the real select below, so a coalesced
	// (latest-wins) value can never overtake a frame this stream's own
	// ordering guarantees actually depend on (turn_started/turn_stopped and
	// the like). Only once cl.send is empty does a pending coalesced value
	// get a turn.
	select {
	case msg := <-cl.send:
		return writeFrame(conn, msg)
	default:
	}
	select {
	case msg := <-cl.send:
		return writeFrame(conn, msg)
	case <-cl.wake:
		for _, msg := range cl.drainPending() {
			if !writeFrame(conn, msg) {
				return false
			}
		}
		return true
	case <-ticker.C:
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		return conn.WriteMessage(websocket.PingMessage, nil) == nil
	case <-cl.done:
		return false
	}
}

func writeFrame(conn *websocket.Conn, msg []byte) bool {
	_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return conn.WriteMessage(websocket.TextMessage, msg) == nil
}
