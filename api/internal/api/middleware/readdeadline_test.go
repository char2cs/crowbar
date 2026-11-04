package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deadlineRecorder is a response writer over a connection: it records every
// read deadline set on it, the way http.ResponseController reaches a conn.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
}

func (r *deadlineRecorder) SetReadDeadline(
	deadline time.Time,
) error {
	r.deadlines = append(r.deadlines, deadline)
	return nil
}

func serveWithDeadlines(
	t *testing.T,
	r *gin.Engine,
	overTLS bool,
) *deadlineRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x", http.NoBody)
	if overTLS {
		req.TLS = &tls.ConnectionState{}
	}
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	r.ServeHTTP(rec, req)
	return rec
}

// TestPreAuthReadDeadline_BoundsATLSRequest proves a request arriving over TLS
// gets a read deadline before any handler runs, timeout from now.
func TestPreAuthReadDeadline_BoundsATLSRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const timeout = 10 * time.Second
	r := gin.New()
	r.Use(PreAuthReadDeadline(timeout))
	r.POST("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	before := time.Now()
	rec := serveWithDeadlines(t, r, true)

	require.Len(t, rec.deadlines, 1)
	assert.WithinRange(t, rec.deadlines[0], before.Add(timeout), time.Now().Add(timeout))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestPreAuthReadDeadline_LeavesPlaintextUnbounded proves the local transport
// (unix socket, loopback dev bind) is never bounded: its trust is the OS's.
func TestPreAuthReadDeadline_LeavesPlaintextUnbounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(PreAuthReadDeadline(time.Second))
	r.POST("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := serveWithDeadlines(t, r, false)

	assert.Empty(t, rec.deadlines)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestLiftReadDeadline_ClearsTheBound proves an authenticated request's read
// deadline is cleared, so a long upload or stream is not cut.
func TestLiftReadDeadline_ClearsTheBound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(PreAuthReadDeadline(time.Second))
	r.POST("/x", func(c *gin.Context) {
		if err := LiftReadDeadline(c); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	rec := serveWithDeadlines(t, r, true)

	require.Len(t, rec.deadlines, 2)
	assert.True(t, rec.deadlines[1].IsZero(), "the lift must clear the deadline, not move it")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestPreAuthReadDeadline_WriterWithoutAConnectionIsServed proves a writer with
// no connection behind it (nothing to bound) does not fail the request.
func TestPreAuthReadDeadline_WriterWithoutAConnectionIsServed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(PreAuthReadDeadline(time.Second))
	r.POST("/x", func(c *gin.Context) {
		if err := LiftReadDeadline(c); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x", http.NoBody)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}
