// Package chats mounts the v0 chat lifecycle REST routes: the flat per-workspace
// list and the create/fork/rename/delete mutations (02 §2.3). Message-send and
// the agent stream are deferred to the Agentic Bridge spike.
package chats

import (
	"time"

	"github.com/gin-gonic/gin"

	chathandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chats/handlers"
)

// Register mounts the chat list, create, fork, rename, and delete routes on the
// supplied router group, backed by the chat lifecycle usecase. The list and
// create routes hang off /workspaces/:wsId/chats while fork, rename, and delete
// address a chat directly under /chats/:id.
func Register(
	rg *gin.RouterGroup,
	chats chathandlers.Lifecycle,
) {
	h := chathandlers.New(chats, time.Now)
	rg.GET("/workspaces/:wsId/chats", h.List)
	rg.POST("/workspaces/:wsId/chats", h.Create)
	rg.POST("/chats/:id/fork", h.Fork)
	rg.PATCH("/chats/:id", h.Rename)
	rg.DELETE("/chats/:id", h.Delete)
}
