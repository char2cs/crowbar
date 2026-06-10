// Package files mounts the v0 file REST routes and the co-located files
// WebSocket upgrade route.
package files

import (
	"github.com/gin-gonic/gin"

	filehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files/handlers"
)

// Register mounts the file content, tree, and mutation routes on the supplied
// router group. filesWS is the WebSocket handler for the live file-change stream.
func Register(
	rg *gin.RouterGroup,
	files filehandlers.Files,
	filesWS gin.HandlerFunc,
) {
	h := filehandlers.New(files)
	rg.GET("/workspaces/:wsId/files/content", h.ReadContent)
	rg.GET("/workspaces/:wsId/files/tree", h.Tree)
	rg.PUT("/workspaces/:wsId/files/content", h.SaveContent)
	rg.POST("/workspaces/:wsId/files", h.Create)
	rg.PATCH("/workspaces/:wsId/files", h.Rename)
	rg.DELETE("/workspaces/:wsId/files", h.Delete)
	rg.GET("/ws/files", filesWS)
}
