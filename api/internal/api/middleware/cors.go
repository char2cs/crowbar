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
