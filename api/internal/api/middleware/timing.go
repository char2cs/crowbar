package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/perf"
)

const serverTimingHeader = "Server-Timing"

// Timing records per-request wall time into the perf ring and echoes it as a
// Server-Timing header, so a capture run can attribute latency from either side
// of the socket without correlating two clocks.
//
// The sample name uses gin's route TEMPLATE (c.FullPath()), not the concrete
// URL: every workspace hits the same handler under a different :wsId, and
// bucketing by concrete path would shatter one hot endpoint into hundreds of
// single-sample names that no aggregation could reassemble. Requests that match
// no route share the "unmatched" bucket for the same reason.
//
// Disabled is the default; the cost is then one atomic load per request, which
// is what lets this wrap every route permanently. While disabled it does not
// wrap the writer or add a header, so an instrumented build is byte-identical
// on the wire to an uninstrumented one.
//
// The ring sample is total wall time. The HEADER can only carry time-to-first-
// byte, because a header set after the handler has written its body never
// reaches the client — the status line went out long before. That is why the
// response writer is wrapped rather than the header simply being set after
// c.Next(): the wrapper stamps the header at the instant the response starts,
// which is the last moment it can still be sent.
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
