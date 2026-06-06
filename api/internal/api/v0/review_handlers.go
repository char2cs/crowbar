package v0

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

// registerReviewHandlers mounts the branch-review REST routes on rg (02 §2.9, 09).
func registerReviewHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.GET("/workspaces/:wsId/review", c.handleGetReview)
	rg.PATCH("/workspaces/:wsId/review", c.handleSetMergeStrategy)
	rg.POST("/workspaces/:wsId/review/threads", c.handleOpenThread)
	rg.POST("/workspaces/:wsId/review/threads/:id/reply", c.handleReplyThread)
	rg.PATCH("/workspaces/:wsId/review/threads/:id", c.handleSetThreadResolved)
}

// handleGetReview GET /v0/workspaces/:wsId/review
func (c *Container) handleGetReview(
	ctx *gin.Context,
) {
	review, err := c.app.Usecases.BranchReview.Get(ctx.Request.Context(), ctx.Param("wsId"))
	if err != nil {
		ctx.JSON(reviewErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, review)
}

// handleSetMergeStrategy PATCH /v0/workspaces/:wsId/review
func (c *Container) handleSetMergeStrategy(
	ctx *gin.Context,
) {
	var body struct {
		MergeStrategy gitdomain.MergeStrategy `json:"mergeStrategy"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := c.app.Usecases.BranchReview.SetMergeStrategy(ctx.Request.Context(), ctx.Param("wsId"), body.MergeStrategy); err != nil {
		ctx.JSON(reviewErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"mergeStrategy": body.MergeStrategy})
}

// handleOpenThread POST /v0/workspaces/:wsId/review/threads
func (c *Container) handleOpenThread(
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
	thread, err := c.app.Usecases.BranchReview.OpenThread(ctx.Request.Context(), branchreview.OpenThreadInput{
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

// handleReplyThread POST /v0/workspaces/:wsId/review/threads/:id/reply
func (c *Container) handleReplyThread(
	ctx *gin.Context,
) {
	var body struct {
		Body string `json:"body"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	thread, err := c.app.Usecases.BranchReview.Reply(ctx.Request.Context(), ctx.Param("id"), body.Body)
	if err != nil {
		ctx.JSON(reviewErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, thread)
}

// handleSetThreadResolved PATCH /v0/workspaces/:wsId/review/threads/:id
func (c *Container) handleSetThreadResolved(
	ctx *gin.Context,
) {
	var body struct {
		IsResolved bool `json:"isResolved"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	thread, err := c.app.Usecases.BranchReview.SetThreadResolved(ctx.Request.Context(), ctx.Param("id"), body.IsResolved)
	if err != nil {
		ctx.JSON(reviewErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, thread)
}
