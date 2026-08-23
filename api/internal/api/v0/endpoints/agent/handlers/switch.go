package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Switch handles POST .../workspaces/:wsId/agent/chats/:id/switch: terminates
// the chat's active provider CLI, assembles a handoff from the ledger, and
// spawns the requested provider as a new segment in the same chat. It
// responds with the new segment's id under the mutation envelope. 404s (via
// requireChatInWorkspace) when id names a chat anchored to a DIFFERENT
// workspace than :wsId.
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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusOK, newSegID)
}

// Resume handles POST .../workspaces/:wsId/agent/chats/:id/resume: revives a
// chat whose vendor CLI is gone (it exited, or it died with the daemon),
// bringing the last provider back into its own native session — exactly where
// the user left it. Responds with the (re)active segment's id; a chat that is
// still live is a no-op that returns its current segment. 404s (via
// requireChatInWorkspace) when id names a chat anchored to a DIFFERENT workspace
// than :wsId.
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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusOK, segID)
}

// Stop handles POST .../workspaces/:wsId/agent/chats/:id/stop: gracefully
// terminates the chat's live vendor CLI and leaves the chat DORMANT and resumable
// — the counterpart of Resume, and what closing a chat TAB calls. The agent
// process stops, but the chat entry (and the conversation it can be resumed into)
// is kept, so reopening the tab revives it via the resume path. The in-flight turn
// is aborted by design ("close = stop"). A chat whose CLI is already gone is a nil
// no-op. 404s (via requireChatInWorkspace) when id names a chat anchored to a
// DIFFERENT workspace than :wsId.
func (h *Handlers) Stop(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	if err := h.runners.StopChat(rctx, id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteAccepted(ctx)
}

// Handoff handles GET .../workspaces/:wsId/agent/chats/:id/handoff: assembles
// the chat's ledger into the legible handoff blob a freshly spawned provider
// CLI can be given as prior context. Used by the `crowbar handoff dump` CLI
// as well as the switch flow internally. 404s (via requireChatInWorkspace)
// when id names a chat anchored to a DIFFERENT workspace than :wsId.
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
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteQueryOK(ctx, dto.HandoffDTO{Handoff: handoff})
}
