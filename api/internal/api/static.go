package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func RegisterStatic(router *gin.Engine, staticFS fs.FS) {
	fileServer := http.FileServer(http.FS(staticFS))

	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/v0/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		// Serve the file; fall back to index.html for SPA routing
		if _, err := staticFS.Open(strings.TrimPrefix(path, "/")); err != nil {
			c.Request.URL.Path = "/"
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
