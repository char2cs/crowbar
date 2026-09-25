package api_test

import (
	"bytes"
	"compress/gzip"
	"io"
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

func gzipped(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write([]byte(s))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func gunzip(t *testing.T, b []byte) string {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(b))
	require.NoError(t, err)
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

func newCompressedStaticFS(t *testing.T) fs.FS {
	t.Helper()
	return fstest.MapFS{
		"index.html":           {Data: []byte("<html>app</html>")},
		"index.html.gz":        {Data: gzipped(t, "<html>app</html>")},
		"assets/app-abc.js":    {Data: []byte("console.log('ok')")},
		"assets/app-abc.js.gz": {Data: gzipped(t, "console.log('ok')")},
		"assets/plain.txt":     {Data: []byte("plain")},
	}
}

func serveStatic(t *testing.T, staticFS fs.FS, target, acceptEncoding string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	crowbarapi.RegisterStatic(router, staticFS)
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	router.ServeHTTP(rec, req)
	return rec
}

func TestRegisterStatic_ServesThePrecompressedSiblingToAGzipClient(t *testing.T) {
	rec := serveStatic(t, newCompressedStaticFS(t), "/assets/app-abc.js", "gzip, deflate, br")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	assert.Equal(t, "Accept-Encoding", rec.Header().Get("Vary"))
	assert.Contains(t, rec.Header().Get("Content-Type"), "javascript")
	assert.Equal(t, "console.log('ok')", gunzip(t, rec.Body.Bytes()))
}

func TestRegisterStatic_ServesIdentityToAClientWithoutGzip(t *testing.T) {
	for _, accept := range []string{"", "br", "gzip;q=0"} {
		rec := serveStatic(t, newCompressedStaticFS(t), "/assets/app-abc.js", accept)

		require.Equal(t, http.StatusOK, rec.Code, accept)
		assert.Empty(t, rec.Header().Get("Content-Encoding"), accept)
		assert.Equal(t, "Accept-Encoding", rec.Header().Get("Vary"), accept)
		assert.Equal(t, "console.log('ok')", rec.Body.String(), accept)
	}
}

func TestRegisterStatic_CompressesTheSPAFallbackIndex(t *testing.T) {
	for _, target := range []string{"/", "/some/spa/route"} {
		rec := serveStatic(t, newCompressedStaticFS(t), target, "gzip")

		require.Equal(t, http.StatusOK, rec.Code, target)
		assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"), target)
		assert.Contains(t, rec.Header().Get("Content-Type"), "text/html", target)
		assert.Equal(t, "<html>app</html>", gunzip(t, rec.Body.Bytes()), target)
		assert.Empty(t, rec.Header().Get("Cache-Control"), target)
	}
}

func TestRegisterStatic_HashedAssetsAreImmutable(t *testing.T) {
	rec := serveStatic(t, newCompressedStaticFS(t), "/assets/plain.txt", "gzip")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
}

func TestRegisterStatic_AMissingAssetFallsBackWithoutImmutableCaching(t *testing.T) {
	rec := serveStatic(t, newCompressedStaticFS(t), "/assets/gone-123.js", "")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("Cache-Control"))
	assert.Contains(t, rec.Body.String(), "app")
}
