package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Get handles GET /v0/chats/:chatId/review, returning the composite branch-review read model.
// It serves the DTO, not the domain object, because a binary file's nil Lines would marshal to
// null and the review pane reads lines.length unguarded; the DTO normalises nil slices to [].
func (h *Handlers) Get(
	ctx *gin.Context,
) {
	review, err := h.reviewUsecase.Get(ctx.Request.Context(), h.workspaceID(ctx))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, dto.BranchReviewDTOFrom(review))
}

// SetMergeStrategy handles PATCH /v0/chats/:chatId/review, updating the
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
	if err := h.reviewUsecase.SetMergeStrategy(ctx.Request.Context(), h.workspaceID(ctx), body.MergeStrategy); err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, gin.H{"mergeStrategy": body.MergeStrategy})
}
