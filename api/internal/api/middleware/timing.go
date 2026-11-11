package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/perf"
)

const serverTimingHeader = "Server-Timing"

// Timing records per-request wall time into the perf ring, keyed by route template
// so per-workspace URLs share one bucket, and echoes time-to-first-byte as
// Server-Timing (the writer is wrapped because a header set after the body is lost).
func Timing() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !perf.Enabled() {
			c.Next()
			return
		}
		w := &timingWriter{ResponseWriter: c.Writer, start: time.Now()}
		c.Writer = w

		c.Next()

		// Bodiless responses have not flushed yet — gin defers their header
		// write until after the whole chain unwinds — so this is their stamp.
		w.stamp()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		perf.Record(fmt.Sprintf("http.%s %s", c.Request.Method, route), time.Since(w.start))
	}
}

// timingWriter stamps the Server-Timing header on the first byte written, the
// last point at which a header can still make it onto the wire.
type timingWriter struct {
	gin.ResponseWriter

	start   time.Time
	stamped bool
}

func (w *timingWriter) WriteHeader(
	code int,
) {
	w.stamp()
	w.ResponseWriter.WriteHeader(code)
}

func (w *timingWriter) WriteHeaderNow() {
	w.stamp()
	w.ResponseWriter.WriteHeaderNow()
}

func (w *timingWriter) Write(
	b []byte,
) (int, error) {
	w.stamp()
	return w.ResponseWriter.Write(b)
}

func (w *timingWriter) WriteString(
	s string,
) (int, error) {
	w.stamp()
	return w.ResponseWriter.WriteString(s)
}

// Unwrap lets http.ResponseController reach the connection's own controls
// (read deadlines) through this wrapper.
func (w *timingWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *timingWriter) stamp() {
	if w.stamped {
		return
	}
	w.stamped = true
	w.Header().Set(
		serverTimingHeader,
		fmt.Sprintf("total;dur=%.1f", float64(time.Since(w.start).Nanoseconds())/1e6),
	)
}
