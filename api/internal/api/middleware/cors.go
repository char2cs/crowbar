package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/origin"
)

const defaultAllowHeaders = "Content-Type"

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
// OPTIONS. A request carrying a disallowed Origin — "null" included — is refused
// 403 whatever its method, before any handler: a simple request (a text/plain
// POST) needs no preflight, so withholding the response alone still lets a page
// fire blind writes. A request with no Origin (CLI, hooks) is not a browser's.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqOrigin := c.GetHeader("Origin")
		if reqOrigin == "" {
			c.Next()
			return
		}

		if !origin.Allowed(reqOrigin, c.Request.Host) {
			c.AbortWithStatus(http.StatusForbidden)
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
