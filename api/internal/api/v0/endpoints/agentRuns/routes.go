package agentRuns

import (
	"github.com/gin-gonic/gin"

	agentrunhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agentRuns/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Register mounts all agent-run routes on rg.
func Register(
	rg *gin.RouterGroup,
	svc usecases.AgentRunUsecase,
) {
	h := agentrunhandlers.New(svc)
	rg.GET("/tasks/:id/agent-runs", h.List)
	rg.POST("/agent-runs/:id/interrupt", h.Interrupt)
}
