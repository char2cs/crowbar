package ws

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// Stream upgrades c's request to a socket of its own and writes each frame of
// frames, in order, until frames closes or the client goes. A frame encode
// gives a close code ends the socket with that code, after the frame itself.
// The socket is kept alive by the pings every socket here sends.
func Stream[T any](c *gin.Context, frames <-chan T, encode func(T) (data []byte, closeCode int)) {
	if !websocket.IsWebSocketUpgrade(c.Request) {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	cl := newClient()
	cl.closing = make(chan int, 1)
	safego.Go("ws.stream.forward", func() { forward(cl, frames, encode) })
	safego.Go("ws.stream.writePump", func() { writePump(conn, cl, nil) })
	readPump(conn)
	cl.closeDone()
}

// forward queues each frame for the write loop, then the close code that ends
// the socket; a source that ends with none ends it at once.
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
