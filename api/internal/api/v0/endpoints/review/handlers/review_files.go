package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// reviewFilesResponse is the data payload of GET /review/files: the full
// changed-files list for the branch (committed + working-tree) with per-file
// +N/-N counts and no line content.
type reviewFilesResponse struct {
	Files []gitdomain.ReviewFileSummary `json:"files"`
}

// GetFiles handles GET /v0/chats/:chatId/review/files, the O(file count) counterpart to Get:
// the complete changed-files list without the line-level diff.
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
