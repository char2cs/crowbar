package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Hooks handles POST .../repos/:repoId/chats/hooks: the vendor-CLI hook forwarder posts a
// canonical hook event here (segment_id/provider/event/payload_raw). The ingress
// records it and answers once its rows are committed; any resulting
// chat-lifecycle change is delivered on the agent-chat WebSocket rather than in
// this response (00 §4).
//
// The response is a bare 202 for every hook but two: one whose stdout the CLI
// must print (a steered prompt), and one that opened a prompt the human can
// answer HERE, which carries an await directive back because that relay must
// stay alive holding the provider's gate open. See AgentHookAckDTO.
func (h *Handlers) Hooks(
	ctx *gin.Context,
) {
	var body struct {
		SegmentID  string `json:"segment_id"`
		Provider   string `json:"provider"`
		Event      string `json:"event"`
		PayloadRaw string `json:"payload_raw"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	reply, err := h.turns.Hook(ctx.Request.Context(), body.SegmentID, body.Provider, body.Event,
		[]byte(body.PayloadRaw), engineagents.ChannelHooks)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	if len(reply.Body) > 0 {
		libs.WriteQueryWithStatus(ctx, http.StatusAccepted, dto.AgentHookAckDTO{Reply: string(reply.Body)})
		return
	}
	// STAY ALIVE: the CLI holds its gate open only for as long as the hook
	// process runs. The relay then long-polls AwaitHookAnswer on the ask.
	if pending, waiting := h.answers.PendingAnswer(reply.Ask); reply.Ask != "" && waiting {
		libs.WriteQueryWithStatus(ctx, http.StatusAccepted, dto.AgentHookAckDTO{
			Await: &dto.AgentHookAwaitDTO{
				ChoiceID: reply.Ask,
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
	reply, err := h.turns.Hook(ctx.Request.Context(), runnerID, ctx.Param("provider"), ctx.Param("event"),
		raw, engineagents.ChannelHooks)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	if len(reply.Body) > 0 {
		ctx.Data(http.StatusOK, "application/json", reply.Body)
		return
	}
	if _, waiting := h.answers.PendingAnswer(reply.Ask); reply.Ask != "" && waiting {
		h.holdForAnswer(ctx, reply.Ask)
		return
	}
	ctx.Status(http.StatusOK)
}

// holdForAnswer parks the request, outside the chat's lock, until askID is decided.
func (h *Handlers) holdForAnswer(
	ctx *gin.Context,
	askID string,
) {
	rctx := ctx.Request.Context()
	answer, err := h.answers.AwaitAnswer(rctx, askID)
	if err != nil {
		if errors.Is(err, rctx.Err()) {
			_ = h.answers.AbandonAnswer(context.WithoutCancel(rctx), askID)
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
