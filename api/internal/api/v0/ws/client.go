package ws

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	pingInterval = 30 * time.Second
	pongTimeout  = 60 * time.Second
	writeTimeout = 10 * time.Second
	sendBuffer   = 64
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
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
	select {
	case msg := <-cl.send:
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		return conn.WriteMessage(websocket.TextMessage, msg) == nil
	case <-ticker.C:
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		return conn.WriteMessage(websocket.PingMessage, nil) == nil
	case <-cl.done:
		return false
	}
}
