package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestCORS_DisallowedOriginGetsNoACAO proves R17: a request from a non-allow-listed
// origin receives NO Access-Control-Allow-Origin header, so the browser blocks a
// malicious site from reading the cross-origin response.
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

// TestRegression_CORS_DisallowedOriginNeverReachesTheHandler proves a page on a
// disallowed origin cannot fire a blind write: a text/plain POST needs no
// preflight, so refusing only OPTIONS still ran the handler. A sandboxed
// iframe's "null" origin is refused the same way.
func TestRegression_CORS_DisallowedOriginNeverReachesTheHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, reqOrigin := range []string{"http://evil.com", "null"} {
		for _, method := range methods {
			t.Run(reqOrigin+" "+method, func(t *testing.T) {
				ran := false
				r := gin.New()
				r.Use(CORS())
				r.Handle(method, "/v0/settings/terminal/profiles", func(c *gin.Context) {
					ran = true
					c.Status(http.StatusOK)
				})

				w := httptest.NewRecorder()
				req := httptest.NewRequestWithContext(t.Context(), method, "/v0/settings/terminal/profiles",
					strings.NewReader(`{"name":"x"}`))
				req.Header.Set("Content-Type", "text/plain")
				req.Header.Set("Origin", reqOrigin)
				r.ServeHTTP(w, req)

				assert.Equal(t, http.StatusForbidden, w.Code)
				assert.False(t, ran, "the handler must not run for a disallowed origin")
			})
		}
	}
}

// TestCORS_AllowedAndOriginlessRequestsReachTheHandler proves the refusal is
// scoped to disallowed origins: the app's own webview, a same-origin page and
// a client sending no Origin at all (CLI, hooks) still get through.
func TestCORS_AllowedAndOriginlessRequestsReachTheHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, reqOrigin := range []string{"", "tauri://localhost", "http://localhost:5173", "https://daemon.example:3737"} {
		t.Run("origin "+reqOrigin, func(t *testing.T) {
			r := gin.New()
			r.Use(CORS())
			r.POST("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

			w := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "https://daemon.example:3737/x", http.NoBody)
			if reqOrigin != "" {
				req.Header.Set("Origin", reqOrigin)
			}
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
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
