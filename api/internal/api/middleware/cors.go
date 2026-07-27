package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/origin"
)

const defaultAllowHeaders = "Content-Type, X-Crowbar-Latency, X-Crowbar-Error-Rate, X-Crowbar-Scenario, X-Crowbar-Fault"

// exposeHeaders names the RESPONSE headers a cross-origin caller may read.
//
// CORS hides every response header from JavaScript except a safelisted handful
// (Content-Type, Content-Length, and four others), and the webview reaches the
// daemon cross-origin — a page on tauri://localhost fetching crowbar://localhost.
// So a header that carries meaning has to be named here or the one client this
// API has reads it back as null.
//
// X-Crowbar-Diff-Truncated is GET /review/patch's report that a capped patch was
// cut short. Unexposed, it fails silently in the worst possible direction: every
// truncated patch reads as a complete one, and the file whose patch was cut
// entirely renders as an empty diff instead of offering to load the rest.
const exposeHeaders = "X-Crowbar-Diff-Truncated"

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
		header.Set("Access-Control-Expose-Headers", exposeHeaders)
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
