package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

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
