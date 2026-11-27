package libs

import (
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const sandboxPolicy = "default-src 'none'; style-src 'unsafe-inline'; sandbox"

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

func WriteUntrustedFile(
	c *gin.Context,
	contentType string,
	data []byte,
) {
	SetUntrustedFileHeaders(c, contentType)
	c.Data(http.StatusOK, contentType, data)
}

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
