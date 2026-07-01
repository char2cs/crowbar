package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/origin"
)

const defaultAllowHeaders = "Content-Type, X-Crowbar-Latency, X-Crowbar-Error-Rate, X-Crowbar-Scenario, X-Crowbar-Fault"

// CORS grants cross-origin access only to allow-listed origins (the in-app Tauri
// webview, loopback dev servers, and any CROWBAR_ALLOWED_ORIGINS entry) — see
// origin.Allowed. It echoes the specific Origin (not "*") so credentialed requests
// are permitted, marks the response Vary: Origin, and short-circuits preflight
// OPTIONS. A disallowed Origin gets NO Access-Control-Allow-Origin header, so the
// browser blocks the cross-origin read; a disallowed preflight is rejected 403.
// This replaces the previous reflect-any-Origin behaviour, which let any website
// the user visited drive the daemon API from their browser.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqOrigin := c.GetHeader("Origin")
		if reqOrigin == "" {
			c.Next()
			return
		}

		if !origin.Allowed(reqOrigin, c.Request.Host) {
			// No ACAO header: the browser blocks reading the cross-origin response.
			// A disallowed preflight is rejected outright rather than handed to a
			// route handler.
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}

		header := c.Writer.Header()
		header.Set("Access-Control-Allow-Origin", reqOrigin)
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
