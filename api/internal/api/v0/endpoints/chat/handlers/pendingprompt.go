package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

func (h *Handlers) PendingPrompt(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	pending, found, err := h.runners.PendingPrompt(ctx.Request.Context(), chat.ID)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	if !found {
		ctx.Status(http.StatusNoContent)
		return
	}
	libs.WriteQueryOK(ctx, dto.PendingPromptDTO{
		Text:      pending.Text,
		State:     pending.State,
		RequestID: pending.RequestID,
	})
}
