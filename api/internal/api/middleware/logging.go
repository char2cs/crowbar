package middleware

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// slowRequest is the duration past which a plain request is worth a Warn.
const slowRequest = 2 * time.Second

// Logger access-logs requests by what a debugger needs: failures (5xx or a
// recorded gin error) at Error, slow requests at Warn, everything else at
// Debug — the UI polls, so Info-level access lines drown the real signal.
// Websocket handshakes (path ends in "/ws", or an Upgrade header) are skipped:
// they reconnect about once a second and gin hijacks them, so their duration
// means nothing.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasSuffix(c.Request.URL.Path, "/ws") ||
			strings.EqualFold(c.Request.Header.Get("Upgrade"), "websocket") {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		took := time.Since(start)
		status := c.Writer.Status()
		attrs := []any{
			"component", "http", "method", c.Request.Method,
			"path", c.Request.URL.Path, "status", status, "took", took,
		}
		ctx := c.Request.Context()
		if errs := c.Errors.Errors(); len(errs) > 0 && status >= 500 {
			slog.ErrorContext(ctx, "http: request failed", append(attrs, "err", strings.Join(errs, "; "))...)
			return
		}
		switch {
		case status >= 500:
			slog.ErrorContext(ctx, "http: request failed", attrs...)
		case took >= slowRequest:
			slog.WarnContext(ctx, "http: slow request", attrs...)
		default:
			slog.DebugContext(ctx, "http: request", attrs...)
		}
	}
}
