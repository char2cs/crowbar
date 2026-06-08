package v0

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// registerAgentRunHandlers mounts the agent-run REST routes on rg.
// /runs/running must be registered before /runs/:id to avoid the static
// segment being swallowed by the wildcard.
func registerAgentRunHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.POST("/workspaces/:wsId/runs", c.handleRunCreate)
	rg.GET("/runs/running", c.handleRunListRunning)
	rg.POST("/runs/:id/start", c.handleRunStart)
	rg.POST("/runs/:id/complete", c.handleRunComplete)
	rg.POST("/runs/:id/fail", c.handleRunFail)
	rg.GET("/runs/:id", c.handleRunGet)
}

// handleRunCreate POST /v0/workspaces/:wsId/runs
func (c *Container) handleRunCreate(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		ID     string `json:"id"`
		ChatID string `json:"chatId"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id := body.ID
	if id == "" {
		id = uuid.NewString()
	}

	run, err := c.app.Repositories.AgentRun.Create(rctx, id, wsID, body.ChatID, time.Now())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, run)
}

// handleRunListRunning GET /v0/runs/running
func (c *Container) handleRunListRunning(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	runs, err := c.app.Repositories.AgentRun.ListRunning(rctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, runs)
}

// handleRunStart POST /v0/runs/:id/start
func (c *Container) handleRunStart(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	run, err := c.app.Repositories.AgentRun.MarkRunning(rctx, id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, run)
}

// handleRunComplete POST /v0/runs/:id/complete
func (c *Container) handleRunComplete(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	run, err := c.app.Repositories.AgentRun.Complete(rctx, id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, run)
}

// handleRunFail POST /v0/runs/:id/fail
func (c *Container) handleRunFail(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	run, err := c.app.Repositories.AgentRun.Fail(rctx, id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, run)
}

// handleRunGet GET /v0/runs/:id
func (c *Container) handleRunGet(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	run, err := c.app.Repositories.AgentRun.Get(rctx, id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	ctx.JSON(http.StatusOK, run)
}
