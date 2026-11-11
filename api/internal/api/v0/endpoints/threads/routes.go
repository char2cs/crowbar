// Package threads mounts the workspace-scoped review-thread routes. Their live changes ride the
// workspace's ws: stream scope.
package threads

import (
	"github.com/gin-gonic/gin"

	threadhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/threads/handlers"
)

// Register mounts the thread routes under the repo-scoped group's /workspaces/:wsId. Every
// mutation broadcasts the resulting ThreadDTO.
func Register(
	repoScoped *gin.RouterGroup,
	store threadhandlers.ThreadStore,
	broadcast threadhandlers.ThreadBroadcaster,
) {
	h := threadhandlers.New(store, broadcast)
	rg := repoScoped.Group("/workspaces/:wsId")
	rg.GET("/threads", h.List)
	rg.POST("/threads", h.OpenThread)
	rg.GET("/threads/:threadId", h.Detail)
	rg.PATCH("/threads/:threadId", h.SetResolved)
	rg.DELETE("/threads/:threadId", h.DeleteThread)
	rg.POST("/threads/:threadId/replies", h.Reply)
	rg.PATCH("/threads/:threadId/messages/:messageId", h.EditMessage)
	rg.DELETE("/threads/:threadId/messages/:messageId", h.DeleteMessage)
}
