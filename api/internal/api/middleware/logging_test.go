package middleware

import (
	"bytes"
	"log"
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
// never reaches log.Printf: files/ws, terminals/:id/ws and every other
// websocket handshake reconnect at ~1/s was 90% of a real daemon.log (10,153
// of 11,254 lines) and drowned out everything else in it.
func TestLogger_SkipsWebsocketRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Logger())
	r.GET("/v0/projects/:id/files/ws", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/v0/projects/:id/git/status", func(c *gin.Context) { c.Status(http.StatusOK) })

	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)

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
