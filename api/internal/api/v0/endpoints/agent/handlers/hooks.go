package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// Hooks handles POST .../workspaces/:wsId/agent/hooks: the vendor-CLI hook forwarder posts a
// canonical hook event here (segment_id/provider/event/payload_raw). IngestHook
// runs the context-move reducer and persists the outcome. Ingestion is a
// fail-fast/good-path-async mutation — the HTTP response is a bare 202, and any
// resulting chat-lifecycle change is delivered later on the agent-chat
// WebSocket rather than in this response (00 §4).
func (h *Handlers) Hooks(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	var body struct {
		DeliveryID string `json:"delivery_id"`
		SegmentID  string `json:"segment_id"`
		Provider   string `json:"provider"`
		Event      string `json:"event"`
		PayloadRaw string `json:"payload_raw"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	var err error
	if body.DeliveryID != "" {
		if deliveries, ok := h.usecase.(interface {
			IngestHookDelivery(
				context.Context,
				string, string, string, string, string,
				[]byte,
			) error
		}); ok {
			err = deliveries.IngestHookDelivery(
				rctx, ctx.Param("wsId"), body.DeliveryID, body.SegmentID,
				body.Provider, body.Event, []byte(body.PayloadRaw),
			)
		} else {
			err = h.usecase.IngestHook(rctx, body.SegmentID, body.Provider, body.Event, []byte(body.PayloadRaw))
		}
	} else {
		err = h.usecase.IngestHook(rctx, body.SegmentID, body.Provider, body.Event, []byte(body.PayloadRaw))
	}
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteAccepted(ctx)
}
