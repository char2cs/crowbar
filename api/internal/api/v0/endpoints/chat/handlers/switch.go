package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Switch handles POST .../chats/:id/switch: quits the current provider's CLI, hands its context off
// and starts the requested provider as a new runner on the same chat, responding with that runner's id.
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
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	newSegID, err := h.runners.SwitchProvider(rctx, id, body.Provider)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusOK, newSegID)
}

// Resume handles POST .../chats/:id/resume: revives a chat whose CLI exited or died with the daemon
// into its last provider's own session, responding with the runner's id (a live chat's is returned as is).
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

// Stop handles POST .../chats/:id/stop: ends the chat's turn, in place where the provider can, else
// with its CLI, leaving the chat dormant and resumable. A chat with no live CLI is a no-op.
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

// Kill handles POST /v0/chats/:chatId/kill: force-ends the chat's CLI even mid-turn, leaving the chat
// dormant and resumable. Any paired client may kill any chat's agent.
func (h *Handlers) Kill(
	ctx *gin.Context,
) {
	if err := h.runners.KillChat(ctx.Request.Context(), ctx.Param("chatId")); err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteNoContent(ctx)
}

// Handoff handles GET .../chats/:id/handoff: the chat's ledger assembled into the handoff blob a newly
// spawned provider gets as prior context (also read by `crowbar handoff dump`).
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

// Compact handles POST .../chats/:id/compact: ask the chat's CLI to compact its own
// context. A provider that declares no compaction gesture is a 404 — the capability is
// absent, not the chat.
func (h *Handlers) Compact(ctx *gin.Context) {
	if err := h.runners.Compact(ctx.Request.Context(), ctx.Param("id")); err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteMutationOK(ctx, http.StatusAccepted, ctx.Param("id"))
}

// SwitchToTerminal handles POST .../chats/:id/switch-to-terminal: moves the chat onto its provider's
// TUI and responds with that terminal session ("" for a dormant chat, which only records the move).
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

// SwitchToNative handles POST .../chats/:id/switch-to-native: moves the chat
// onto Crowbar's own chat surface; a chat already there is a no-op.
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
