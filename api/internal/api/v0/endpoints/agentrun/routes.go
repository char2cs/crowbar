// Package agentrun mounts the v0 agent-run REST routes.
// /runs/running must be registered before /runs/:id to avoid the static
// segment being swallowed by the wildcard.
package agentrun

import (
	"github.com/gin-gonic/gin"

	agentrunhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agentrun/handlers"
)

// Register mounts the agent-run routes on the supplied router group.
func Register(rg *gin.RouterGroup, repo agentrunhandlers.AgentRunRepo) {
	h := agentrunhandlers.New(repo)

	rg.POST("/workspaces/:wsId/runs", h.Create)
	rg.GET("/runs/running", h.ListRunning)
	rg.POST("/runs/:id/start", h.Start)
	rg.POST("/runs/:id/complete", h.Complete)
	rg.POST("/runs/:id/fail", h.Fail)
	rg.GET("/runs/:id", h.Get)
}
