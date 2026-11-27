package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

func (h *Handlers) GetFileSides(
	ctx *gin.Context,
) {
	path := ctx.Query("path")
	if path == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path query parameter is required")
		return
	}
	sides, err := h.reviewUsecase.GetFileSides(
		ctx.Request.Context(),
		h.workspaceID(ctx),
		scopeCommit(ctx),
		path,
		ctx.Query("oldPath"),
	)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, sides)
}
