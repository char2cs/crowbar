package v0_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
)

func TestProviderHandlers_NoEngine_503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	// Nil out provider to simulate unavailable engine.
	tc.eng.Provider = nil
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))

	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws1/provider", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestProtectedBranches_NoEngine_503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	tc.eng.Provider = nil
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))

	req := httptest.NewRequest(http.MethodGet, "/v0/repos/r1/protected-branches?path=/tmp", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestProtectedBranches_WorkspaceNotFound_404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))

	// Workspace "r1" does not exist in the store → 404.
	req := httptest.NewRequest(http.MethodGet, "/v0/repos/r1/protected-branches", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestProviderState_WorkspaceNotFound_404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tc := newApp(t)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))

	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/nonexistent/provider", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
