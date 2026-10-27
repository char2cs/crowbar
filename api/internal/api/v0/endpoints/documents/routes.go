// Package documents mounts the routes that read and write a file by document
// ref (a workspace's file, or a host file outside every worktree) and the
// socket a client holds a host document's watch by.
package documents

import (
	"github.com/gin-gonic/gin"
)

// Register mounts GET and PUT /documents/content and GET /documents/ws on rg,
// with stream serving the watch once its gate passes. It sits on the flat /v0
// group: a host document belongs to no chat or workspace to scope it by.
func Register(
	rg *gin.RouterGroup,
	files Documents,
	stream gin.HandlerFunc,
) {
	h := &handlers{docs: files, stream: stream}
	rg.GET("/documents/content", h.Read)
	rg.PUT("/documents/content", h.Write)
	rg.GET("/documents/ws", h.Watch)
}
