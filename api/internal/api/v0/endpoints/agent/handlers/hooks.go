package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// hookDeliveryIngress is the journalled ingress: the relay mints one delivery id
// and reuses it on every retry, and this path turns those retries into ONE
// semantic hook. It is discovered rather than declared on AgentUsecase because a
// hand-assembled test usecase legitimately has no journal, and a hook with no
// delivery id (the daemon's own replay) takes the plain path either way.
type hookDeliveryIngress interface {
	IngestHookDelivery(
		ctx context.Context,
		workspaceID, deliveryID, runnerID, provider, canonicalEvent string,
		rawPayload []byte,
	) error
}

// ingest routes one hook to the journalled ingress when both a journal and a
// delivery id exist, and to the plain one otherwise. Exactly one of the two runs:
// they are the same ingestion, and running both would apply every effect twice.
func (h *Handlers) ingest(
	ctx context.Context,
	workspaceID, deliveryID, runnerID, provider, event string,
	raw []byte,
) error {
	if deliveries, journalled := h.usecase.(hookDeliveryIngress); journalled && deliveryID != "" {
		return deliveries.IngestHookDelivery(
			ctx, workspaceID, deliveryID, runnerID, provider, event, raw)
	}
	return h.usecase.IngestHook(ctx, runnerID, provider, event, raw)
}

// Hooks handles POST .../workspaces/:wsId/agent/hooks: the vendor-CLI hook forwarder posts a
// canonical hook event here (segment_id/provider/event/payload_raw). IngestHook
// runs the context-move reducer and persists the outcome. Ingestion is a
// fail-fast/good-path-async mutation — any resulting chat-lifecycle change is
// delivered later on the agent-chat WebSocket rather than in this response
// (00 §4).
//
// The response is a bare 202 for every hook but one: a hook that opened a prompt
// the human can answer HERE carries an await directive back, because that relay
// must stay alive holding the provider's gate open. See AgentHookAckDTO.
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

	err := h.ingest(rctx, ctx.Param("wsId"), body.DeliveryID,
		body.SegmentID, body.Provider, body.Event, []byte(body.PayloadRaw))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	// One instruction can come back on the acknowledgement: STAY ALIVE. A hook that
	// opened a prompt this provider can be told the answer to must not exit, because
	// the CLI holds its gate open only for as long as the hook process runs. The
	// relay then long-polls AwaitHookAnswer on the same delivery id.
	//
	// A hook that opened nothing answerable — every hook of a provider with no
	// answer channel, and most hooks of one that has — gets a bare 202 and exits, as
	// it always did.
	if pending, waiting := h.usecase.PendingAnswer(body.DeliveryID); waiting {
		libs.WriteQueryWithStatus(ctx, http.StatusAccepted, dto.AgentHookAckDTO{
			Await: &dto.AgentHookAwaitDTO{
				ChoiceID: pending.ChoiceID,
				WaitMS:   pending.Wait.Milliseconds(),
			},
		})
		return
	}
	libs.WriteAccepted(ctx)
}
