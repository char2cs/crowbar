package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A plaintext (local socket / loopback) request still gets pprof, so the
// desktop watchdog can capture a goroutine dump from a wedged daemon.
func TestMountDebug_ServesGoroutineDump(
	t *testing.T,
) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	mountDebug(router)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine?debug=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "goroutine profile:")
}

// TestRegression_MountDebug_RefusesTLS proves pprof is local-only: a request
// that arrived over the network (TLS) is refused before any profile runs,
// whatever its method or credentials.
func TestRegression_MountDebug_RefusesTLS(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	mountDebug(router)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/debug/pprof/"},
		{http.MethodGet, "/debug/pprof/cmdline"},
		{http.MethodGet, "/debug/pprof/goroutine?debug=2"},
		{http.MethodGet, "/debug/pprof/profile?seconds=1"},
		{http.MethodPost, "/debug/pprof/symbol"},
	} {
		req := httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, http.NoBody)
		req.TLS = &tls.ConnectionState{}
		req.Header.Set("Authorization", "Bearer any-token")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equalf(t, http.StatusForbidden, w.Code, "%s %s over TLS must be refused", tc.method, tc.path)
		assert.NotContainsf(t, w.Body.String(), "goroutine", "%s %s leaked profile data", tc.method, tc.path)
	}
}

func TestMountDebug_ServesIndex(
	t *testing.T,
) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	mountDebug(router)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Types of profiles available")
}
