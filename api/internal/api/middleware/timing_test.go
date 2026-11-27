package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/perf"
)

func newTimingRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Timing())
	r.GET("/v0/workspaces/:wsId/review/files", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

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

func TestTiming_ConnectionControlsReachThroughTheWrapper(
	t *testing.T,
) {
	perf.SetEnabled(true)
	t.Cleanup(func() {
		perf.SetEnabled(false)
		perf.Reset()
	})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Timing())
	r.GET("/x", func(c *gin.Context) {
		if err := http.NewResponseController(c.Writer).SetReadDeadline(time.Time{}); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.Status(http.StatusOK)
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/x", http.NoBody)
	require.NoError(t, err)
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))
}

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
