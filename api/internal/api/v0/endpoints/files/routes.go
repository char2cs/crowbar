// Package files mounts the v0 file explorer and file-content REST routes: the
// lazy tree read, the read/save content pair, and the create/rename/delete
// filesystem mutations (02 §2.4). Every route hangs off
// /workspaces/:wsId/files; the two reads carry their path in the ?path= query
// because GET has no body, while the mutations take it in the JSON body.
package files

import (
	"time"

	"github.com/gin-gonic/gin"

	filehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files/handlers"
)

// Register mounts the file tree, content read/save, and create/rename/delete
// routes on the supplied router group, backed by the file usecase.
func Register(
	rg *gin.RouterGroup,
	files filehandlers.Files,
) {
	h := filehandlers.New(files, time.Now)
	rg.GET("/workspaces/:wsId/files/tree", h.Tree)
	rg.GET("/workspaces/:wsId/files/content", h.ReadContent)
	rg.PUT("/workspaces/:wsId/files/content", h.SaveContent)
	rg.POST("/workspaces/:wsId/files", h.Create)
	rg.PATCH("/workspaces/:wsId/files", h.Rename)
	rg.DELETE("/workspaces/:wsId/files", h.Delete)
}
