package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/perf"
)

// newTimingRouter mounts a parameterised route so a test can prove the sample
// name collapses per-workspace URLs into their shared template.
func newTimingRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Timing())
	r.GET("/v0/workspaces/:wsId/review/files", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

// TestTiming_RecordsRouteTemplateNotConcretePath pins the bucketing rule: two
// different workspaces hitting the same handler must land in ONE sample name.
// Recording the concrete path would shatter a hot endpoint into hundreds of
// single-sample buckets that no aggregation could reassemble.
func TestTiming_RecordsRouteTemplateNotConcretePath(
	t *testing.T,
) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() {
		perf.SetEnabled(false)
		perf.Reset()
	})

	r := newTimingRouter()
	for _, ws := range []string{"ws-a", "ws-b"} {
		req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/"+ws+"/review/files", http.NoBody)
		r.ServeHTTP(httptest.NewRecorder(), req)
	}

	samples := perf.Snapshot()
	assert.Len(t, samples, 2)
	for _, s := range samples {
		assert.Equal(t, "http.GET /v0/workspaces/:wsId/review/files", s.Name)
	}
}

// TestTiming_RecordsNothingWhenDisabled proves the permanently-installed
// middleware costs one atomic load while disarmed, which is what lets it wrap
// every v0 route in production.
func TestTiming_RecordsNothingWhenDisabled(
	t *testing.T,
) {
	perf.Reset()
	perf.SetEnabled(false)

	r := newTimingRouter()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws-a/review/files", http.NoBody)
	r.ServeHTTP(httptest.NewRecorder(), req)

	assert.Empty(t, perf.Snapshot())
}

// TestTiming_RecordsUnmatchedRoutesUnderOneBucket proves a 404 — which has no
// route template — does not record an empty name, and does not fan out one
// bucket per bogus URL a scanner might probe.
func TestTiming_RecordsUnmatchedRoutesUnderOneBucket(
	t *testing.T,
) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() {
		perf.SetEnabled(false)
		perf.Reset()
	})

	r := newTimingRouter()
	for _, path := range []string{"/nope", "/also-nope"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, http.NoBody))
	}

	samples := perf.Snapshot()
	assert.Len(t, samples, 2)
	for _, s := range samples {
		assert.Equal(t, "http.GET unmatched", s.Name)
	}
}

// TestTiming_EmitsServerTimingHeaderOnTheWire asserts against rec.Result(),
// NOT rec.Header(). The recorder's live header map keeps accepting writes long
// after the response has been flushed, so asserting on it would pass for a
// header no client could ever receive; Result() carries the snapshot taken at
// WriteHeader, which is what actually goes out.
func TestTiming_EmitsServerTimingHeaderOnTheWire(
	t *testing.T,
) {
	perf.Reset()
	perf.SetEnabled(true)
	t.Cleanup(func() {
		perf.SetEnabled(false)
		perf.Reset()
	})

	r := newTimingRouter()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws-a/review/files", http.NoBody)
	r.ServeHTTP(rec, req)

	assert.Contains(t, rec.Result().Header.Get("Server-Timing"), "total;dur=")
}

// TestTiming_OmitsServerTimingHeaderWhenDisabled proves the disarmed path adds
// no header, so an instrumented build is indistinguishable from an
// uninstrumented one until a capture run arms it.
func TestTiming_OmitsServerTimingHeaderWhenDisabled(
	t *testing.T,
) {
	perf.Reset()
	perf.SetEnabled(false)

	r := newTimingRouter()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws-a/review/files", http.NoBody)
	r.ServeHTTP(rec, req)

	assert.Empty(t, rec.Result().Header.Get("Server-Timing"))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
}
