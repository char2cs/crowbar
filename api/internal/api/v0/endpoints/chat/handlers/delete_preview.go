package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// deletePreviewResponse is the body of GET .../chats/:id/delete-preview: what
// deleting id would take, without taking it.
type deletePreviewResponse struct {
	ChatCount int `json:"chatCount"`
	FileCount int `json:"fileCount"`
}

// DeletePreview handles GET .../repos/:repoId/chats/:id/delete-preview,
// backing an idle delete's confirm dialog: "N uncommitted files, M chats".
//
// id may name a CHAT or a FOLDER. The chat count is cheap either way — a
// client-side tree walk already answers it, per the addendum spec this route
// comes from — but the file count is not: once a subtree can span more than
// one independent workspace, showing it means summing git status across all
// of them, which is exactly what h.folders.DeletePreview does. 404s on an
// unknown id.
func (h *Handlers) DeletePreview(
	ctx *gin.Context,
) {
	chatCount, fileCount, err := h.folders.DeletePreview(ctx.Request.Context(), ctx.Param("id"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteQueryOK(ctx, deletePreviewResponse{ChatCount: chatCount, FileCount: fileCount})
}
