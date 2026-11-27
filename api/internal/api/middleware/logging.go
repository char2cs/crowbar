package middleware

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const slowRequest = 2 * time.Second

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
		if status >= 500 && ctx.Err() != nil {
			slog.DebugContext(ctx, "http: client went away", attrs...)
			return
		}
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
