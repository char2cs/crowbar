package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestCORS_DisallowedOriginGetsNoACAO proves R17: a request from a non-allow-listed
// origin is still served (the daemon doesn't enforce CSRF for same-origin/native
// callers) but receives NO Access-Control-Allow-Origin header, so the browser
// blocks a malicious site from reading the cross-origin response.
func TestCORS_DisallowedOriginGetsNoACAO(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "http://evil.com")
	r.ServeHTTP(w, req)

	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"),
		"a disallowed origin must not receive an Access-Control-Allow-Origin header")
}

// TestCORS_DisallowedPreflightRejected proves a preflight from a non-allow-listed
// origin is rejected 403 rather than handed to a route handler.
func TestCORS_DisallowedPreflightRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS())
	r.POST("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "http://evil.com")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

// TestCORS_AllowedTauriOriginEchoed proves the in-app webview origin is allowed
// and echoed back with credentials.
func TestCORS_AllowedTauriOriginEchoed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "tauri://localhost")
	r.ServeHTTP(w, req)

	assert.Equal(t, "tauri://localhost", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}
