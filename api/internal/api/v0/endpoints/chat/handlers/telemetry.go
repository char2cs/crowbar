package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

func (h *Handlers) Telemetry(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	report, ok := h.turns.Telemetry(chat.ID)
	// Absence, never a dead control: a chat on a surface whose channel
	// carries no usage report has a number that can never move again, and
	// serving the last one it earned elsewhere is worse than no gauge.
	if !ok || !h.turns.TelemetryOnChatSurface(ctx.Request.Context(), chat.ID) {
		ctx.Status(http.StatusNoContent)
		return
	}

	out := dto.AgentTelemetryDTOFrom(report)
	libs.WriteQueryOK(ctx, out)
}
