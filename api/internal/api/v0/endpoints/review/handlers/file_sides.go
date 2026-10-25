package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// GetFileSides handles GET /v0/chats/:chatId/review/contents?path=&oldPath=,
// serving one file's full old and new text so the reader can reveal the
// unchanged lines a patch leaves out.
//
// It is answered per file and only on request — never as part of the outline or
// the patch — so a branch pays for it only where a reader expands. oldPath is
// the rename source and may be omitted.
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
		libs.WriteErr(ctx, reviewErrorStatus(err), err.Error())
		return
	}
	libs.WriteQueryOK(ctx, sides)
}
