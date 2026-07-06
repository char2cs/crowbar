// Package agent mounts the v0 /v0/agent REST and WebSocket routes: agentic-chat
// lifecycle CRUD, the vendor-CLI hook ingestion endpoint, and the agent-chat
// lifecycle WebSocket (00 agentic-engine spec).
package agent

import (
	"github.com/gin-gonic/gin"

	agenthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agent/handlers"
)

// Register mounts the /v0/agent REST routes and the agent-chat lifecycle
// WebSocket upgrade route on the supplied top-level router group. Unlike the
// entity-scoped surfaces, /v0/agent is NOT nested under
// /projects/:projectId/repos/:repoId/workspaces/:wsId: an AgentChat carries its
// own workspaceId, so the routes stay flat on rg (mirroring health/system).
func Register(
	rg *gin.RouterGroup,
	usecase agenthandlers.AgentUsecase,
	wsHandle gin.HandlerFunc,
) {
	h := agenthandlers.New(usecase)

	rg.POST("/agent/chats", h.Create)
	rg.GET("/agent/chats", h.List)
	rg.GET("/agent/chats/:id", h.Get)
	rg.POST("/agent/chats/:id/switch", h.Switch)
	rg.GET("/agent/chats/:id/handoff", h.Handoff)
	rg.POST("/agent/hooks", h.Hooks)
	rg.GET("/agent/ws/chats", wsHandle)
}
