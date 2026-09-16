package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// SetChatPermissionLevel handles
// PUT .../repos/:repoId/chats/:id/permission-level.
func (h *Handlers) SetChatPermissionLevel(
	ctx *gin.Context,
) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	var body struct {
		Level string `json:"level"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	err := h.answers.SetChatPermissionLevel(ctx.Request.Context(), chat.ID, body.Level)
	if err != nil {
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}
	libs.WriteMutationOK(ctx, http.StatusOK, chat.ID)
}
