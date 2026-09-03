package middleware

import (
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger access-logs every request except a websocket handshake (path ends in
// "/ws" — files/ws, terminals/:id/ws, chats/ws, lsp/ws, ...). A live daemon.log
// was 90% (10,153 of 11,254 lines) reconnect spam from a single files/ws
// client polling roughly once a second, which buried every other line in it.
// Its "duration" was also meaningless for a route gin hijacks into a
// long-lived socket rather than returning from.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasSuffix(c.Request.URL.Path, "/ws") {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		log.Printf("%s %s %d %s", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(start))
	}
}
