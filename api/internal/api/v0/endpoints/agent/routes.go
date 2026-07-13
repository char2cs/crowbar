// Package agent mounts the workspace-scoped .../workspaces/:wsId/agent REST and
// WebSocket routes: agentic-chat lifecycle CRUD, the vendor-CLI hook ingestion
// endpoint, and the agent-chat lifecycle WebSocket (00 agentic-engine spec;
// Task 3 nested the surface under the workspace group).
package agent

import (
	"github.com/gin-gonic/gin"

	agenthandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agent/handlers"
)

// Register mounts the agent REST routes and the agent-chat lifecycle WebSocket
// upgrade route on the supplied workspace-scoped router group (wsScoped, i.e.
// .../projects/:projectId/repos/:repoId/workspaces/:wsId), mirroring
// terminal.Register. Every AgentChat is anchored to exactly one workspace, so
// nesting here gives Create/List/Get/Switch/Rename/Handoff/Delete the :wsId
// path param the handlers scope by, and gives the whole surface
// scopeWorkspaceToPath's wsId-ownership enforcement (router.go) for free. The
// WS route lands in the SAME group so its :wsId is available to
// agentChatDef's Filter (container.go) — the resulting route is
// .../workspaces/:wsId/agent/ws/chats.
//
// .../agent/runners/:segid/rename is keyed by the RUNNER, not the chat: it
// resolves runnerID → runner → CurrentChatID at call time (RenameByRunner), so
// the `crowbar chat rename --segment <segid>` CLI a spawned agent invokes can
// never carry a chat id baked in at spawn — the exact staleness a /clear or
// /resume moving the runner between chats used to produce.
func Register(
	wsScoped *gin.RouterGroup,
	usecase agenthandlers.AgentUsecase,
	wsHandle gin.HandlerFunc,
) {
	h := agenthandlers.New(usecase)

	wsScoped.POST("/agent/chats", h.Create)
	wsScoped.GET("/agent/chats", h.List)
	wsScoped.GET("/agent/chats/:id", h.Get)
	wsScoped.POST("/agent/chats/:id/switch", h.Switch)
	wsScoped.POST("/agent/chats/:id/resume", h.Resume)
	wsScoped.POST("/agent/chats/:id/rename", h.Rename)
	wsScoped.GET("/agent/chats/:id/handoff", h.Handoff)
	wsScoped.DELETE("/agent/chats/:id", h.Delete)
	wsScoped.POST("/agent/runners/:segid/rename", h.RenameByRunner)
	wsScoped.POST("/agent/hooks", h.Hooks)
	wsScoped.GET("/agent/providers", h.Providers)
	wsScoped.GET("/agent/ws/chats", wsHandle)
}
