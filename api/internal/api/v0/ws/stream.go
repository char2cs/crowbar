package ws

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// Stream serves one socket from frames on the shared write loop and its
// pings; a frame encoded with a close code ends the socket after itself.
func Stream[T any](c *gin.Context, frames <-chan T, encode func(T) (data []byte, closeCode int)) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // Upgrade already answered: 400 for no handshake, 403 for a foreign origin
	}
	cl := newClient()
	cl.closing = make(chan int, 1)
	safego.Go("ws.stream.forward", func() { forward(cl, frames, encode) })
	safego.Go("ws.stream.writePump", func() { writePump(conn, cl, nil) })
	readPump(conn)
	cl.closeDone()
}

// forward queues frames for the write loop; a source that closes with no
// close code ends the socket at once.
func forward[T any](cl *client, frames <-chan T, encode func(T) ([]byte, int)) {
	for f := range frames {
		data, code := encode(f)
		select {
		case cl.send <- data:
		case <-cl.done:
			return
		}
		if code != 0 {
			cl.closing <- code
			return
		}
	}
	cl.closeDone()
}

// writeClose writes what is queued, then the close frame with code.
func writeClose(conn *websocket.Conn, cl *client, code int) bool {
	if !writeQueued(conn, cl) {
		return false
	}
	msg := websocket.FormatCloseMessage(code, "")
	if err := conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(writeTimeout)); err != nil {
		slog.Debug("ws: stream: close", "code", code, "err", err)
	}
	return false
}
