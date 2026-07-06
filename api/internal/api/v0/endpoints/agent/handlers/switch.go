package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Switch handles POST /v0/agent/chats/:id/switch: terminates the chat's active
// provider CLI, assembles a handoff from the ledger, and spawns the requested
// provider as a new segment in the same chat. It responds with the new
// segment's id under the mutation envelope.
func (h *Handlers) Switch(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	var body struct {
		Provider string `json:"provider"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	newSegID, err := h.usecase.SwitchProvider(rctx, id, body.Provider)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusOK, newSegID)
}

// Handoff handles GET /v0/agent/chats/:id/handoff: assembles the chat's
// ledger into the legible handoff blob a freshly spawned provider CLI can be
// given as prior context. Used by the `crowbar handoff dump` CLI as well as
// the switch flow internally.
func (h *Handlers) Handoff(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	handoff, err := h.usecase.AssembleHandoff(rctx, id)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteQueryOK(ctx, dto.HandoffDTO{Handoff: handoff})
}
