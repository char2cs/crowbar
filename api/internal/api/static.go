package api

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

func RegisterStatic(router *gin.Engine, staticFS fs.FS) {
	fileServer := http.FileServer(http.FS(staticFS))

	router.NoRoute(func(c *gin.Context) {
		urlPath := c.Request.URL.Path

		if strings.HasPrefix(urlPath, "/api/") || strings.HasPrefix(urlPath, "/v0/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		name := strings.TrimPrefix(urlPath, "/")
		if name == "" {
			name = "index.html"
		} else if _, err := fs.Stat(staticFS, name); err != nil {
			c.Request.URL.Path = "/"
			name = "index.html"
		}

		if strings.HasPrefix(urlPath, "/assets/") && c.Request.URL.Path != "/" {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}

		preferGzipSibling(c, staticFS, name)
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}

func preferGzipSibling(c *gin.Context, staticFS fs.FS, name string) {
	info, err := fs.Stat(staticFS, name+".gz")
	if err != nil || info.IsDir() {
		return
	}
	c.Header("Vary", "Accept-Encoding")
	if !acceptsGzip(c.Request) {
		return
	}
	if ctype := mime.TypeByExtension(path.Ext(name)); ctype != "" {
		c.Header("Content-Type", ctype)
	}
	c.Header("Content-Encoding", "gzip")
	c.Request.URL.Path = "/" + name + ".gz"
}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		coding, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(coding), "gzip") {
			continue
		}
		q := strings.ReplaceAll(strings.TrimSpace(params), " ", "")
		return q != "q=0" && q != "q=0.0" && q != "q=0.00" && q != "q=0.000"
	}
	return false
}
