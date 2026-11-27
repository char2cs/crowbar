package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/origin"
)

const defaultAllowHeaders = "Content-Type"

const exposeHeaders = "X-Crowbar-Diff-Truncated"

func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		if answerCORS(c.Writer, c.Request) {
			c.Abort()
			return
		}
		c.Next()
	}
}

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
