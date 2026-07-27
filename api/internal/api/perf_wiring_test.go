package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	crowbarapi "github.com/char2cs/crowbar/api/internal/api"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/internal/perf"
)

// newRealAPI builds the production HTTP surface — the same middleware stack and
// the same v0 route tree the daemon serves. The perf wiring is only worth
// anything if it is reachable HERE: a handler that answers on a hand-built test
// router but mounts at the wrong path in the real container fails silently, and
// a capture run against the live daemon is what would discover it.
func newRealAPI(
	t *testing.T,
) *crowbarapi.Container {
	t.Helper()
	ctx := context.Background()
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	a, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)
	t.Cleanup(a.Close)

	c, err := crowbarapi.New(a, eng, nil)
	require.NoError(t, err)
	return c
}

// TestAPI_New_PerfRoutesMountedUnderV0 reads the real router's route table so
// the assertion is about REGISTRATION, not about a request happening to 200.
func TestAPI_New_PerfRoutesMountedUnderV0(
	t *testing.T,
) {
	router, ok := newRealAPI(t).Handler().(*gin.Engine)
	require.True(t, ok, "the api container serves a gin engine")

	mounted := map[string]struct{}{}
	for _, r := range router.Routes() {
		mounted[r.Method+" "+r.Path] = struct{}{}
	}

	assert.Contains(t, mounted, "GET /v0/system/perf")
	assert.Contains(t, mounted, "POST /v0/system/perf")
}

// TestAPI_New_PerfRouteAnswersOnRealRouter walks the whole production chain —
// every global middleware plus the v0 group's guards — to the perf handler.
func TestAPI_New_PerfRouteAnswersOnRealRouter(
	t *testing.T,
) {
	perf.Reset()
	perf.SetEnabled(false)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/system/perf", http.NoBody)
	newRealAPI(t).Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Success bool `json:"success"`
		Data    struct {
			Enabled bool          `json:"enabled"`
			Samples []perf.Sample `json:"samples"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.True(t, got.Success)
	assert.False(t, got.Data.Enabled)
	assert.Empty(t, got.Data.Samples)
}

// TestAPI_New_CaptureRunRoundTrip walks the whole capture protocol over HTTP —
// arm, drive a request, read the ring back, disarm — against the production
// router. Each piece passing in isolation proves nothing about the run a perf
// investigation actually performs: an unwired middleware still passes its own
// unit tests, and records nothing here.
func TestAPI_New_CaptureRunRoundTrip(
	t *testing.T,
) {
	handler := newRealAPI(t).Handler()
	t.Cleanup(func() {
		perf.SetEnabled(false)
		perf.Reset()
	})

	arm := httptest.NewRecorder()
	handler.ServeHTTP(arm, httptest.NewRequest(http.MethodPost, "/v0/system/perf?enabled=true", http.NoBody))
	require.Equal(t, http.StatusOK, arm.Code)
	require.True(t, perf.Enabled())

	driven := httptest.NewRecorder()
	handler.ServeHTTP(driven, httptest.NewRequest(http.MethodGet, "/v0/health", http.NoBody))
	require.Equal(t, http.StatusOK, driven.Code)
	assert.Contains(t, driven.Result().Header.Get("Server-Timing"), "total;dur=")

	read := httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/v0/system/perf", http.NoBody))
	require.Equal(t, http.StatusOK, read.Code)

	var got struct {
		Data struct {
			Enabled bool          `json:"enabled"`
			Samples []perf.Sample `json:"samples"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(read.Body.Bytes(), &got))
	assert.True(t, got.Data.Enabled)

	names := []string{}
	for _, s := range got.Data.Samples {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "http.GET /v0/health")
	// The arming request itself must NOT appear: it was disarmed when the
	// middleware saw it, and Set clears the ring anyway. A capture window that
	// opened with its own opening request already inside it would attribute
	// setup cost to the scenario under measurement.
	assert.NotContains(t, names, "http.POST /v0/system/perf")

	disarm := httptest.NewRecorder()
	handler.ServeHTTP(disarm, httptest.NewRequest(http.MethodPost, "/v0/system/perf?enabled=false", http.NoBody))
	require.Equal(t, http.StatusOK, disarm.Code)
	assert.False(t, perf.Enabled())
	assert.Empty(t, perf.Snapshot())
}
