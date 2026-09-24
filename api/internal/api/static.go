package api

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// RegisterStatic serves the embedded web bundle, falling back to index.html for
// SPA routes.
//
// Precompressed siblings: when the bundle carries `<file>.gz` next to a file
// (api/Makefile's embed-web writes them at build time) and the client accepts
// gzip, the sibling is served with Content-Encoding: gzip — no compression work
// at request time. Vite's content-hashed `/assets/*` never change under a name,
// so they are cached as immutable.
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
			// Unknown path: an SPA route, served index.html.
			c.Request.URL.Path = "/"
			name = "index.html"
		}

		if strings.HasPrefix(urlPath, "/assets/") && c.Request.URL.Path != "/" {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}

		if info, err := fs.Stat(staticFS, name+".gz"); err == nil && !info.IsDir() {
			c.Header("Vary", "Accept-Encoding")
			if acceptsGzip(c.Request) {
				if ctype := mime.TypeByExtension(path.Ext(name)); ctype != "" {
					c.Header("Content-Type", ctype)
				}
				c.Header("Content-Encoding", "gzip")
				c.Request.URL.Path = "/" + name + ".gz"
			}
		}

		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}

// acceptsGzip reports whether the request's Accept-Encoding lists gzip with a
// non-zero quality.
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
