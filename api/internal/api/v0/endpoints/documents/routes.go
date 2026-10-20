// Package documents mounts the one route that reads and writes a file by
// document ref: a workspace's file, or a host file outside every worktree.
package documents

import (
	"github.com/gin-gonic/gin"
)

// Register mounts GET and PUT /documents/content on rg. It sits on the flat
// /v0 group: a host document belongs to no chat or workspace to scope it by.
func Register(
	rg *gin.RouterGroup,
	files Documents,
) {
	h := &handlers{docs: files}
	rg.GET("/documents/content", h.Read)
	rg.PUT("/documents/content", h.Write)
}
