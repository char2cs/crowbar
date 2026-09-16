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

// GetFiles handles GET /v0/chats/:chatId/review/files, returning the
// files-only branch-review summary. It is the cheap, O(file count) counterpart
// to Get: the sidebar uses it to show the complete changed-files list without
// pulling the line-level branch diff that Get's read model carries.
func (h *Handlers) GetFiles(
	ctx *gin.Context,
) {
	files, err := h.reviewUsecase.GetFiles(ctx.Request.Context(), h.workspaceID(ctx), scopeCommit(ctx))
	if err != nil {
		libs.WriteErr(ctx, reviewErrorStatus(err), err.Error())
		return
	}
	if files == nil {
		files = []gitdomain.ReviewFileSummary{}
	}
	libs.WriteQueryOK(ctx, reviewFilesResponse{Files: files})
}
