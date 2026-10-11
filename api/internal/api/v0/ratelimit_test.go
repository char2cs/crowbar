package v0

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClock is the limiter's injected clock; tests move it by hand.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func limitedEngine(
	l *sourceLimiter,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(l.handle)
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func hitFrom(
	t *testing.T,
	r *gin.Engine,
	remote string,
) int {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

// TestSourceLimiter_RejectsOverMaxPerSource proves the (max+1)th request from
// one source within a window is rejected with 429, while another source keeps
// its own full budget.
func TestSourceLimiter_RejectsOverMaxPerSource(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_000, 0)}
	r := limitedEngine(newSourceLimiter(3, time.Minute, clock.now))

	for i := range 3 {
		require.Equalf(t, http.StatusOK, hitFrom(t, r, "203.0.113.9:1000"), "request %d", i)
	}
	assert.Equal(t, http.StatusTooManyRequests, hitFrom(t, r, "203.0.113.9:1001"),
		"a new port is the same source")
	for i := range 3 {
		assert.Equalf(t, http.StatusOK, hitFrom(t, r, "198.51.100.7:2000"), "other source, request %d", i)
	}
}

// TestSourceLimiter_ResetsAfterWindow proves an exhausted source stays limited
// for the whole window and regains its budget once the window elapses.
func TestSourceLimiter_ResetsAfterWindow(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_000, 0)}
	r := limitedEngine(newSourceLimiter(1, 10*time.Second, clock.now))

	require.Equal(t, http.StatusOK, hitFrom(t, r, "203.0.113.9:1000"))
	require.Equal(t, http.StatusTooManyRequests, hitFrom(t, r, "203.0.113.9:1000"))

	clock.t = clock.t.Add(10 * time.Second)
	require.Equal(t, http.StatusTooManyRequests, hitFrom(t, r, "203.0.113.9:1000"), "still inside the window")
	clock.t = clock.t.Add(time.Nanosecond)
	assert.Equal(t, http.StatusOK, hitFrom(t, r, "203.0.113.9:1000"))
}

// TestSourceLimiter_IgnoresForwardedHeaders proves a caller cannot pick a fresh
// budget by naming another address in a header.
func TestSourceLimiter_IgnoresForwardedHeaders(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_000, 0)}
	r := limitedEngine(newSourceLimiter(1, time.Minute, clock.now))

	codes := make([]int, 0, 2)
	for i := range 2 {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
		req.RemoteAddr = "203.0.113.9:1000"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.0.%d", i))
		req.Header.Set("X-Real-IP", fmt.Sprintf("10.0.1.%d", i))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
	}
	assert.Equal(t, []int{http.StatusOK, http.StatusTooManyRequests}, codes)
}

// TestSourceLimiter_KeysIPv6ByItsSlash64 proves addresses inside one /64 share
// a budget, since a single host can rotate through all of them.
func TestSourceLimiter_KeysIPv6ByItsSlash64(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_000, 0)}
	r := limitedEngine(newSourceLimiter(1, time.Minute, clock.now))

	require.Equal(t, http.StatusOK, hitFrom(t, r, "[2001:db8:1:2::1]:1000"))
	assert.Equal(t, http.StatusTooManyRequests, hitFrom(t, r, "[2001:db8:1:2:ffff::9]:1000"))
	assert.Equal(t, http.StatusOK, hitFrom(t, r, "[2001:db8:1:3::1]:1000"), "another /64 is another source")
}

// TestSourceLimiter_TableIsBounded proves the per-source table never grows past
// maxLimitedSources, however many distinct sources arrive within one window.
func TestSourceLimiter_TableIsBounded(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_000, 0)}
	l := newSourceLimiter(1, time.Minute, clock.now)
	r := limitedEngine(l)

	for i := range maxLimitedSources + 10 {
		clock.t = clock.t.Add(time.Millisecond)
		require.Equal(t, http.StatusOK, hitFrom(t, r, fmt.Sprintf("10.%d.%d.%d:1", i>>16&0xff, i>>8&0xff, i&0xff)))
	}
	assert.Len(t, l.sources, maxLimitedSources)
	assert.NotContains(t, l.sources, "10.0.0.0", "the oldest window is the one evicted")
}
