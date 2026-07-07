package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The daemon listens on a private unix socket only, so the pprof surface is
// reachable exclusively by local processes that can already open the socket —
// it exists so the desktop watchdog (and a developer with curl) can capture a
// goroutine dump from a live-but-wedged daemon.
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
