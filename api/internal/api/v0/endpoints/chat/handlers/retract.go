package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Retract handles POST .../chats/:id/prompts/:requestId/retract: Esc after a
// send. It answers what became of the prompt and its text, for the composer.
func (h *Handlers) Retract(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	got, err := h.runners.Retract(ctx.Request.Context(), chat.ID, ctx.Param("requestId"))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteQueryOK(ctx, dto.AgentRetractDTO{State: got.State, Text: got.Text})
}
