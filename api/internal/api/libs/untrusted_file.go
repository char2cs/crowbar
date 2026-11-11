package libs

import (
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// sandboxPolicy denies a navigated-to file every capability: the daemon serves the web app on the
// same origin, so a script it ran could call every /v0 endpoint. Inline style only keeps SVG legible.
const sandboxPolicy = "default-src 'none'; style-src 'unsafe-inline'; sandbox"

// SetUntrustedFileHeaders marks a response whose bytes came from a user, an agent or a repo. Only
// types a browser renders without running script stay inline; the rest download into a sandbox.
func SetUntrustedFileHeaders(
	c *gin.Context,
	contentType string,
) {
	c.Header("X-Content-Type-Options", "nosniff")
	if rendersInert(contentType) {
		c.Header("Content-Disposition", "inline")
		return
	}
	c.Header("Content-Disposition", "attachment")
	c.Header("Content-Security-Policy", sandboxPolicy)
}

// WriteUntrustedFile answers 200 with data as contentType under SetUntrustedFileHeaders.
func WriteUntrustedFile(
	c *gin.Context,
	contentType string,
	data []byte,
) {
	SetUntrustedFileHeaders(c, contentType)
	c.Data(http.StatusOK, contentType, data)
}

// rendersInert reports the types the web shows through <img>, <audio>, <video> or the PDF viewer.
// A PDF is kept off the sandbox because a sandboxed document cannot load the browser's viewer.
func rendersInert(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch mediaType {
	case "image/svg+xml":
		return false
	case "application/pdf":
		return true
	}
	return strings.HasPrefix(mediaType, "image/") ||
		strings.HasPrefix(mediaType, "audio/") ||
		strings.HasPrefix(mediaType, "video/")
}
