package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
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
		libs.WriteErr(ctx, reviewErrorStatus(err), err.Error())
		return
	}
	libs.WriteQueryOK(ctx, review)
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
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.reviewUsecase.SetMergeStrategy(ctx.Request.Context(), ctx.Param("wsId"), body.MergeStrategy); err != nil {
		libs.WriteErr(ctx, reviewErrorStatus(err), err.Error())
		return
	}
	libs.WriteQueryOK(ctx, gin.H{"mergeStrategy": body.MergeStrategy})
}
