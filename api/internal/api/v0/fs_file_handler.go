package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// FsFile serves the contents of a single file as a JSON string. The desktop
// daemon has no real filesystem read wired up yet; this mock echoes the
// requested path so the editor has something to display.
func FsFile(c *gin.Context) {
	path := c.Query("path")
	c.JSON(http.StatusOK, "// "+path+"\n// Mock file content served by the Go daemon.\n")
}
