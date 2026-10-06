package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestLogger_PassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Logger())
	r.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestLogger_SkipsWebsocketRoutes proves a request whose path ends in "/ws"
// is never access-logged: files/ws, terminals/:id/ws and every other
// websocket handshake reconnect at ~1/s was 90% of a real daemon.log (10,153
// of 11,254 lines) and drowned out everything else in it.
func TestLogger_SkipsWebsocketRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Logger())
	r.GET("/v0/projects/:id/files/ws", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/v0/projects/:id/git/status", func(c *gin.Context) { c.Status(http.StatusOK) })

	buf := captureSlog(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/v0/projects/abc/files/ws", nil)
	r.ServeHTTP(w, req)
	assert.Empty(t, buf.String(), "a /ws route must not be access-logged")

	buf.Reset()
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/v0/projects/abc/git/status", nil)
	r.ServeHTTP(w, req)
	assert.NotEmpty(t, buf.String(), "a normal route must still be access-logged")
}

// TestLogger_SkipsWebsocketUpgrades pins that a websocket handshake is never
// access-logged whatever its path: /v0/console/logs holds its socket open for
// minutes, so logging it put a "slow request" Warn into the very console that
// streams it every time it reconnected.
func TestLogger_SkipsWebsocketUpgrades(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Logger())
	r.GET("/v0/console/logs", func(c *gin.Context) { c.Status(http.StatusOK) })

	buf := captureSlog(t)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v0/console/logs", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	r.ServeHTTP(httptest.NewRecorder(), req)
	assert.Empty(t, buf.String(), "a websocket upgrade must not be access-logged")
}

func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return &buf
}

// TestLogger_LevelFollowsOutcome pins that a routine request is Debug while a
// 5xx is Error, so a default-level reader sees failures and not polling.
func TestLogger_LevelFollowsOutcome(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Logger())
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/boom", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	buf := captureSlog(t)
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ok", nil))
	assert.Contains(t, buf.String(), "level=DEBUG")

	buf.Reset()
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/boom", nil))
	assert.Contains(t, buf.String(), "level=ERROR")
}
