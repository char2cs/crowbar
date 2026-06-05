package ws

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Dispatch serves the same URL as both a REST snapshot and a live WebSocket: a
// WebSocket upgrade request streams via the Broadcaster, any other request gets
// the JSON snapshot body (03 §1a).
func Dispatch[T any](
	b *Broadcaster[T],
	snapshot func(*gin.Context) (any, error),
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if websocket.IsWebSocketUpgrade(c.Request) {
			b.Handle(c)
			return
		}
		writeSnapshot(c, snapshot)
	}
}

func writeSnapshot(
	c *gin.Context,
	snapshot func(*gin.Context) (any, error),
) {
	body, err := snapshot(c)
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, body)
}
