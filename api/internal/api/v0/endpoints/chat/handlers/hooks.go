package handlers

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const runnerTokenHeader = "X-Crowbar-Runner-Token"

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
