package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type reviewFilesResponse struct {
	Files []gitdomain.ReviewFileSummary `json:"files"`
}

func (h *Handlers) GetFiles(
	ctx *gin.Context,
) {
	files, err := h.reviewUsecase.GetFiles(ctx.Request.Context(), h.workspaceID(ctx), scopeCommit(ctx))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	if files == nil {
		files = []gitdomain.ReviewFileSummary{}
	}
	libs.WriteQueryOK(ctx, reviewFilesResponse{Files: files})
}
