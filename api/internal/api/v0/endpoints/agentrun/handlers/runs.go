package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Create handles POST /v0/workspaces/:wsId/runs.
func (h *Handlers) Create(ctx *gin.Context) {
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

	run, err := h.repo.Create(rctx, id, wsID, body.ChatID, time.Now())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, run)
}

// ListRunning handles GET /v0/runs/running.
func (h *Handlers) ListRunning(ctx *gin.Context) {
	rctx := ctx.Request.Context()

	runs, err := h.repo.ListRunning(rctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, runs)
}

// Start handles POST /v0/runs/:id/start.
func (h *Handlers) Start(ctx *gin.Context) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	run, err := h.repo.MarkRunning(rctx, id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, run)
}

// Complete handles POST /v0/runs/:id/complete.
func (h *Handlers) Complete(ctx *gin.Context) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	run, err := h.repo.Complete(rctx, id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, run)
}

// Fail handles POST /v0/runs/:id/fail.
func (h *Handlers) Fail(ctx *gin.Context) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	run, err := h.repo.Fail(rctx, id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, run)
}

// Get handles GET /v0/runs/:id.
func (h *Handlers) Get(ctx *gin.Context) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	run, err := h.repo.Get(rctx, id)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	ctx.JSON(http.StatusOK, run)
}
