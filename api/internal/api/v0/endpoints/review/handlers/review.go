package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// reviewErrorStatus maps a usecase error to an HTTP status: a genuine
// not-found entity yields 404, every other (internal) failure yields 500, so a
// real git/subprocess error is never masked as a 404.
func reviewErrorStatus(
	err error,
) int {
	if errors.Is(err, apperr.ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// Get handles GET /v0/workspaces/:wsId/review, returning the composite
// branch-review read model for the workspace.
func (h *Handlers) Get(
	ctx *gin.Context,
) {
	review, err := h.reviewUsecase.Get(ctx.Request.Context(), ctx.Param("wsId"))
	if err != nil {
		ctx.JSON(reviewErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, review)
}

// SetMergeStrategy handles PATCH /v0/workspaces/:wsId/review, updating the
// merge strategy for the workspace.
func (h *Handlers) SetMergeStrategy(
	ctx *gin.Context,
) {
	var body struct {
		MergeStrategy gitdomain.MergeStrategy `json:"mergeStrategy"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.reviewUsecase.SetMergeStrategy(ctx.Request.Context(), ctx.Param("wsId"), body.MergeStrategy); err != nil {
		ctx.JSON(reviewErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"mergeStrategy": body.MergeStrategy})
}

// OpenThread handles POST /v0/workspaces/:wsId/review/threads, opening a new
// review thread anchored to a file location.
func (h *Handlers) OpenThread(
	ctx *gin.Context,
) {
	var body struct {
		FilePath   string            `json:"filePath"`
		LineNumber int               `json:"lineNumber"`
		Side       domain.ReviewSide `json:"side"`
		Body       string            `json:"body"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	thread, err := h.reviewUsecase.OpenThread(ctx.Request.Context(), branchreview.OpenThreadInput{
		WsID:       ctx.Param("wsId"),
		FilePath:   body.FilePath,
		LineNumber: body.LineNumber,
		Side:       body.Side,
		Body:       body.Body,
	})
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, thread)
}

// Reply handles POST /v0/workspaces/:wsId/review/threads/:id/reply, appending
// a reply message to an existing review thread.
func (h *Handlers) Reply(
	ctx *gin.Context,
) {
	var body struct {
		Body string `json:"body"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	thread, err := h.reviewUsecase.Reply(ctx.Request.Context(), ctx.Param("id"), body.Body)
	if err != nil {
		ctx.JSON(reviewErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, thread)
}

// SetThreadResolved handles PATCH /v0/workspaces/:wsId/review/threads/:id,
// marking a review thread resolved or reopening it.
func (h *Handlers) SetThreadResolved(
	ctx *gin.Context,
) {
	var body struct {
		IsResolved bool `json:"isResolved"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	thread, err := h.reviewUsecase.SetThreadResolved(ctx.Request.Context(), ctx.Param("id"), body.IsResolved)
	if err != nil {
		ctx.JSON(reviewErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, thread)
}
