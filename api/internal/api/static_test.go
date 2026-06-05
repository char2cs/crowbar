package api_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crowbarapi "github.com/char2cs/crowbar/api/internal/api"
)

func newStaticFS(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"index.html": {Data: []byte("<html>app</html>")},
		"app.js":     {Data: []byte("console.log('ok')")},
	}
}

func TestRegisterStatic_APIPrefix_Returns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	crowbarapi.RegisterStatic(router, newStaticFS(t))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/anything", nil)
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "not found")
}

func TestRegisterStatic_V0Prefix_Returns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	crowbarapi.RegisterStatic(router, newStaticFS(t))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v0/anything", nil)
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "not found")
}

func TestRegisterStatic_KnownFile_Served(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	crowbarapi.RegisterStatic(router, newStaticFS(t))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/app.js", nil)
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "console.log")
}

func TestRegisterStatic_UnknownPath_FallsBackToIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	crowbarapi.RegisterStatic(router, newStaticFS(t))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/some/spa/route", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "app")
}
