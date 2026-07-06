package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// Hooks handles POST /v0/agent/hooks: the vendor-CLI hook forwarder posts a
// canonical hook event here (segment_id/event/payload). IngestHook runs the
// context-move reducer and persists the outcome. Ingestion is a fail-fast/
// good-path-async mutation — the HTTP response is a bare 202, and any
// resulting chat-lifecycle change is delivered later on the agent-chat
// WebSocket rather than in this response (00 §4).
func (h *Handlers) Hooks(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	var body struct {
		SegmentID string         `json:"segment_id"`
		Event     string         `json:"event"`
		Payload   map[string]any `json:"payload"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.usecase.IngestHook(rctx, body.SegmentID, body.Event, body.Payload); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteAccepted(ctx)
}
