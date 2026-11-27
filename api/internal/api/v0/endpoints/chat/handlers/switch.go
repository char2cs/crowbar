package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

func (h *Handlers) Switch(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	var body struct {
		Provider string `json:"provider"`
		Surface  string `json:"surface"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if !knownSurface(body.Surface) {
		libs.WriteErr(ctx, http.StatusBadRequest, "unknown surface: "+body.Surface)
		return
	}

	newSegID, err := h.runners.SwitchProvider(rctx, id, body.Provider, body.Surface)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusOK, newSegID)
}

func (h *Handlers) Resume(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	segID, err := h.runners.ResumeChat(rctx, id)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusOK, segID)
}

func (h *Handlers) Stop(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	if err := h.runners.StopChat(rctx, id); err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteNoContent(ctx)
}

func (h *Handlers) Kill(
	ctx *gin.Context,
) {
	if err := h.runners.KillChat(ctx.Request.Context(), ctx.Param("chatId")); err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteNoContent(ctx)
}

func (h *Handlers) Handoff(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	handoff, err := h.chats.AssembleHandoff(rctx, id)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteQueryOK(ctx, dto.HandoffDTO{Handoff: handoff})
}

func (h *Handlers) Compact(ctx *gin.Context) {
	if err := h.runners.Compact(ctx.Request.Context(), ctx.Param("id")); err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteMutationOK(ctx, http.StatusAccepted, ctx.Param("id"))
}

func (h *Handlers) StopShell(ctx *gin.Context) {
	if err := h.runners.StopShell(ctx.Request.Context(), ctx.Param("id"), ctx.Param("shellId")); err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteMutationOK(ctx, http.StatusAccepted, ctx.Param("shellId"))
}

func (h *Handlers) SwitchToTerminal(ctx *gin.Context) {
	id := ctx.Param("id")
	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}
	termSessID, err := h.runners.SwitchToTerminal(ctx.Request.Context(), id)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteMutationOK(ctx, http.StatusOK, termSessID)
}

func (h *Handlers) SwitchToNative(ctx *gin.Context) {
	id := ctx.Param("id")
	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}
	if err := h.runners.SwitchToNative(ctx.Request.Context(), id); err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteNoContent(ctx)
}
