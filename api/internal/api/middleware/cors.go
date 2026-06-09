package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const defaultAllowHeaders = "Content-Type, X-Crowbar-Latency, X-Crowbar-Error-Rate, X-Crowbar-Scenario, X-Crowbar-Fault"

// CORS reflects the request Origin so the browser-served dev frontend can call
// the daemon cross-origin (Vite on :5173 → daemon on :3737). It reflects the
// Origin rather than using "*" so credentialed requests are permitted, and
// short-circuits preflight OPTIONS with 204. The daemon binds localhost only,
// so reflective CORS does not widen any real attack surface.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		header := c.Writer.Header()
		header.Set("Access-Control-Allow-Origin", origin)
		header.Set("Access-Control-Allow-Credentials", "true")
		header.Add("Vary", "Origin")

		if c.Request.Method == http.MethodOptions {
			writePreflight(c)
			return
		}

		c.Next()
	}
}

func writePreflight(
	c *gin.Context,
) {
	header := c.Writer.Header()
	header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

	requested := c.GetHeader("Access-Control-Request-Headers")
	if requested == "" {
		requested = defaultAllowHeaders
	}
	header.Set("Access-Control-Allow-Headers", requested)

	c.AbortWithStatus(http.StatusNoContent)
}
