package handlers

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// runnerTokenHeader carries the per-runner token an HTTP hook proves itself with.
const runnerTokenHeader = "X-Crowbar-Runner-Token"

// HTTPHook handles POST .../chats/runners/:segid/hooks/:provider/:event, the one hook ingress,
// authenticated by the runner token. A hook that opened a prompt a human can decide is held open and
// answered with the decision; the CLI closing the request means it was decided at the terminal.
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
		libs.Fail(ctx, err)
		return
	}
	if len(reply.Body) > 0 {
		ctx.Data(http.StatusOK, "application/json", reply.Body)
		return
	}
	if reply.Ask != "" && h.answers.HoldsAnswer(reply.Ask) {
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
			return
		}
		libs.Fail(ctx, err)
		return
	}
	if len(answer.Stdout) == 0 {
		ctx.Status(http.StatusOK)
		return
	}
	ctx.Data(http.StatusOK, "application/json", answer.Stdout)
}
