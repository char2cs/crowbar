package v0

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFixedWindowLimiter_AllowsUpToMax proves the first max requests within a
// window all succeed.
func TestFixedWindowLimiter_AllowsUpToMax(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(newFixedWindowLimiter(3, time.Minute))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))
		assert.Equal(t, http.StatusOK, rec.Code, "request %d", i)
	}
}

// TestFixedWindowLimiter_RejectsOverMax proves the (max+1)th request within
// the same window is rejected with 429.
func TestFixedWindowLimiter_RejectsOverMax(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(newFixedWindowLimiter(3, time.Minute))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 3; i++ {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))

	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "expected 429 on the 4th request within the window")
}

// TestFixedWindowLimiter_ResetsAfterWindow proves a caller who has exhausted
// the window regains capacity once the window elapses. The window itself is
// wall-clock-based, so this waits on the real clock — bounded by
// require.Eventually's deadline, never a bare time.Sleep — rather than faking
// it, since the thing under test IS the passage of real time.
func TestFixedWindowLimiter_ResetsAfterWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(newFixedWindowLimiter(1, 10*time.Millisecond))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))
	rejected := httptest.NewRecorder()
	r.ServeHTTP(rejected, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))
	require.Equal(t, http.StatusTooManyRequests, rejected.Code, "expected the 2nd immediate request to be rate-limited")

	require.Eventually(t, func() bool {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil))
		return rec.Code == http.StatusOK
	}, time.Second, 2*time.Millisecond, "limiter never reset within 1s of a 10ms window")
}
