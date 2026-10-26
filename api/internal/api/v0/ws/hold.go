package ws

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/char2cs/crowbar/api/internal/core/safego"
)

// Hold serves a WebSocket whose only meaning is that it is open: it carries no
// frames, and the returned release runs when it closes, however it closes.
//
// acquire runs BEFORE the handshake completes, so the 101 is the server's
// statement that the hold is already counted (see Broadcaster.Handle).
func Hold(
	c *gin.Context,
	acquire func() (release func()),
) {
	if !websocket.IsWebSocketUpgrade(c.Request) {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	release := acquire()
	defer release()

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	defer func() { _ = conn.Close() }()

	done := make(chan struct{})
	defer close(done)
	safego.Go("hold.ping", func() { pingUntil(conn, done) })
	readPump(conn)
}

// pingUntil keeps the peer's pong deadline (readPump) fed, so a vanished peer
// ends the hold instead of lingering.
func pingUntil(
	conn *websocket.Conn,
	done <-chan struct{},
) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if conn.WriteMessage(websocket.PingMessage, nil) != nil {
				return
			}
		}
	}
}
