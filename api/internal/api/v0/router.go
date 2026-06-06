package v0

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/health"
)

// Register mounts the v0 REST and WebSocket routes.
func (c *Container) Register(
	rg *gin.RouterGroup,
) {
	health.Register(rg)
	rg.GET("/ws/workspaces", c.workspaces.Handle)
	rg.GET("/ws/chats", c.chats.Handle)
	rg.GET("/ws/git", c.git.Handle)
	rg.GET("/ws/files", c.files.Handle)
	rg.GET("/ws/lsp", c.lsp.Handle)
	registerTerminalHandlers(rg, c)
	registerSearchHandlers(rg, c)
	registerProviderHandlers(rg, c)
	registerReviewHandlers(rg, c)
	registerLSPHandlers(rg, c)
}
