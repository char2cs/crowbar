package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit caps every request body so a single unbounded upload can't exhaust
// daemon memory (a trivial DoS, acute when the dev TCP transport is bound). It
// wraps the body in http.MaxBytesReader: once a handler reads past maxBytes the
// read fails, the connection is closed, and the bind error surfaces as a 4xx
// rather than the daemon buffering the whole stream. WebSocket upgrades and other
// bodiless requests pass through untouched.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}
