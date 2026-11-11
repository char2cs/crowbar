package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/origin"
)

const defaultAllowHeaders = "Content-Type"

// exposeHeaders names the response headers the webview may read: it fetches the
// daemon cross-origin, and CORS hides every non-safelisted header from JavaScript.
// Without X-Crowbar-Diff-Truncated a truncated review patch would read as complete.
const exposeHeaders = "X-Crowbar-Diff-Truncated"

// CORS echoes allow-listed origins (origin.Allowed) so credentialed requests work,
// and refuses a disallowed Origin ("null" included) 403 before any handler: a simple
// POST needs no preflight, so withholding only the response would still allow blind writes.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		if answerCORS(c.Writer, c.Request) {
			c.Abort()
			return
		}
		c.Next()
	}
}

// CORSHandler is CORS in front of a handler the API's router does not serve:
// the boot gate, whose answers the webview reads cross-origin too.
func CORSHandler(
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if answerCORS(w, r) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

// answerCORS sets the headers r's Origin is granted and reports whether it
// answered r itself: a granted preflight, or any request from a refused Origin.
func answerCORS(
	w http.ResponseWriter,
	r *http.Request,
) bool {
	reqOrigin := r.Header.Get("Origin")
	if reqOrigin == "" {
		return false
	}

	if !origin.Allowed(reqOrigin, r.Host) {
		w.WriteHeader(http.StatusForbidden)
		return true
	}

	header := w.Header()
	header.Set("Access-Control-Allow-Origin", reqOrigin)
	header.Set("Access-Control-Allow-Credentials", "true")
	header.Set("Access-Control-Expose-Headers", exposeHeaders)
	header.Add("Vary", "Origin")

	if r.Method == http.MethodOptions {
		writePreflight(w, r)
		return true
	}
	return false
}

func writePreflight(
	w http.ResponseWriter,
	r *http.Request,
) {
	header := w.Header()
	header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

	requested := r.Header.Get("Access-Control-Request-Headers")
	if requested == "" {
		requested = defaultAllowHeaders
	}
	header.Set("Access-Control-Allow-Headers", requested)

	w.WriteHeader(http.StatusNoContent)
}
