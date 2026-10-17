package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCORS_PreflightShortCircuits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS())
	r.POST("/x", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Headers", "X-Requested-With")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "PATCH")
	assert.Equal(t, "X-Requested-With", w.Header().Get("Access-Control-Allow-Headers"))
}

func TestCORS_ActualRequestReflectsOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS())
	r.GET("/x", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "Origin", w.Header().Get("Vary"))
}

// The webview reaches the daemon cross-origin (a page on tauri://localhost
// fetching crowbar://localhost), so CORS governs which RESPONSE headers its
// JavaScript may read. Only the safelisted handful are readable by default:
// without an explicit Access-Control-Expose-Headers, GET /review/patch's
// X-Crowbar-Diff-Truncated reads back as null and every truncated patch is
// silently mistaken for a complete one.
func TestCORS_ExposesDiffTruncatedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS())
	r.GET("/x", func(c *gin.Context) {
		c.Header("X-Crowbar-Diff-Truncated", "true")
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "tauri://localhost")
	r.ServeHTTP(w, req)

	assert.Contains(t, w.Header().Get("Access-Control-Expose-Headers"), "X-Crowbar-Diff-Truncated")
}

func TestCORS_NoOriginPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS())
	r.GET("/x", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_PreflightWithoutRequestHeadersFallsBack(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS())
	r.POST("/x", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "Content-Type", w.Header().Get("Access-Control-Allow-Headers"))
}

// A handler outside the router gets the router's CORS: the boot gate answers
// the webview before the API exists.
func TestCORSHandler_GrantsWhatTheRouterGrants(t *testing.T) {
	gate := CORSHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	serve := func(method, origin string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), method, "/v0/projects", http.NoBody)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		gate.ServeHTTP(w, req)
		return w
	}

	allowed := serve(http.MethodGet, "tauri://localhost")
	preflight := serve(http.MethodOptions, "tauri://localhost")
	foreign := serve(http.MethodOptions, "https://evil.example")
	plain := serve(http.MethodGet, "")

	assert.Equal(t, http.StatusServiceUnavailable, allowed.Code)
	assert.Equal(t, "tauri://localhost", allowed.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusNoContent, preflight.Code)
	assert.Contains(t, preflight.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Equal(t, http.StatusForbidden, foreign.Code)
	assert.Empty(t, foreign.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusServiceUnavailable, plain.Code)
	assert.Empty(t, plain.Header().Get("Access-Control-Allow-Origin"))
}
