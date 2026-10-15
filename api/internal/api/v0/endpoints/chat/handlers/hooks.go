package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// ingest routes one hook by whether it carries a delivery id: with one, through
// the delivery-id dedup, which turns a relay's retries into ONE semantic hook;
// without one — the daemon's own replay — straight through. Exactly one of the
// two runs: they are the same ingestion, and running both would apply every
// effect twice.
//
// Both ingresses are declared on TurnUsecase, so which one exists is settled at
// COMPILE time. The journalled half used to be discovered with a `, ok` type
// assertion, and that failed silently: a port that stopped carrying it sent every
// hook down the un-journalled path, and every retry duplicated the user turn and
// the assistant message it had already applied.
func (h *Handlers) ingest(
	ctx context.Context,
	workspaceID, deliveryID, runnerID, provider, event string,
	raw []byte,
) error {
	if deliveryID == "" {
		return h.turns.IngestHook(ctx, runnerID, provider, event, raw)
	}
	return h.turns.IngestHookDelivery(
		ctx, deliveryID, runnerID, provider, event, raw,
	)
}

// Hooks handles POST .../repos/:repoId/chats/hooks: the vendor-CLI hook forwarder posts a
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

	// ctx.Param("wsId") is "" at the repo-scoped mount (Task 17): Turns.
	// hookDeliveryScope already falls back to the runner's own persisted
	// WorkspaceID whenever the route supplies none, so this only loses the
	// narrow runner-not-yet-persisted race the route value used to shortcut.
	// The home mount still injects a real :wsId and keeps that fast path.
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
	if reply, ok := h.turns.TakeHookReply(body.DeliveryID); ok && body.DeliveryID != "" {
		libs.WriteQueryWithStatus(ctx, http.StatusAccepted, dto.AgentHookAckDTO{Reply: reply})
		return
	}
	if pending, waiting := h.answers.PendingAnswer(body.DeliveryID); waiting {
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

// runnerTokenHeader carries the per-runner token an HTTP hook proves itself with.
const runnerTokenHeader = "X-Crowbar-Runner-Token"

// HTTPHook handles POST .../chats/runners/:segid/hooks/:provider/:event: a vendor
// CLI's own HTTP hook, which posts the provider's payload as the body with no relay
// process in between. The runner, provider and event come from the path where the
// relay put them in an envelope, and the runner token stands where the relay's
// access to the socket did.
//
// The answer channel is the response itself. A hook that opened a prompt a human can
// decide here is held open until they do, and the answer is the body — the relay's
// await long-poll, with the CLI as the one waiting. The CLI closing its request is
// the signal the relay used to send as SIGTERM: the prompt was decided at the
// terminal, so it is abandoned.
func (h *Handlers) HTTPHook(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	runnerID := ctx.Param("segid")
	if !h.providers.VerifyRunnerToken(runnerID, ctx.GetHeader(runnerTokenHeader)) {
		libs.WriteErr(ctx, http.StatusUnauthorized, "invalid runner token")
		return
	}
	raw, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	deliveryID := uuid.NewString()
	if err := h.ingest(rctx, ctx.Param("wsId"), deliveryID,
		runnerID, ctx.Param("provider"), ctx.Param("event"), raw); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	if reply, ok := h.turns.TakeHookReply(deliveryID); ok {
		ctx.Data(http.StatusOK, "application/json", []byte(reply))
		return
	}
	if _, waiting := h.answers.PendingAnswer(deliveryID); waiting {
		h.holdForAnswer(ctx, deliveryID)
		return
	}
	ctx.Status(http.StatusOK)
}

// holdForAnswer parks the request until the prompt deliveryID opened is decided.
func (h *Handlers) holdForAnswer(
	ctx *gin.Context,
	deliveryID string,
) {
	rctx := ctx.Request.Context()
	answer, err := h.answers.AwaitAnswer(rctx, deliveryID)
	if err != nil {
		if errors.Is(err, rctx.Err()) {
			_ = h.answers.AbandonAnswer(context.WithoutCancel(rctx), deliveryID)
			return
		}
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	if len(answer.Stdout) == 0 {
		ctx.Status(http.StatusOK)
		return
	}
	ctx.Data(http.StatusOK, "application/json", answer.Stdout)
}
