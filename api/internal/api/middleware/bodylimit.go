package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit caps every request body at maxBytes so one unbounded upload can't
// exhaust daemon memory: reading past the cap fails the bind with a 4xx instead
// of buffering the whole stream. Bodiless requests (WebSocket upgrades) pass through.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}
