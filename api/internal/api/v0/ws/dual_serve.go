package ws

import (
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// DualServe serves one URL as both a REST endpoint and a WebSocket stream. A
// request carrying a WebSocket upgrade is routed to wsHandler; every other
// request is served by rest. It mirrors the per-endpoint dispatch pattern so an
// endpoint's Register can wire a single GET that upgrades to the live
// broadcaster on demand (02 §1, §2.2). It uses the same upgrade detection as the
// dedicated WS dispatcher so behaviour stays consistent across both paths.
func DualServe(
	rest gin.HandlerFunc,
	wsHandler gin.HandlerFunc,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if websocket.IsWebSocketUpgrade(c.Request) {
			wsHandler(c)
			return
		}
		rest(c)
	}
}
