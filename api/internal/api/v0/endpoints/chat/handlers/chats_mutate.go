package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// Rename handles POST .../chats/:id/rename. `?source=agent` yields to a user-locked title; the default
// (a human rename) sets the title unconditionally and locks it.
func (h *Handlers) Rename(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")
	source := ctx.Query("source")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	var body struct {
		Title string `json:"title"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.chats.RenameChat(rctx, id, body.Title, source); err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteNoContent(ctx)
}

// SetSelection handles PATCH .../chats/:id/selection with the WHOLE model+effort pair (empty clears to
// the provider default): valid efforts depend on the model, so a partial write could store an invalid pair.
func (h *Handlers) SetSelection(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	var body struct {
		Model  string `json:"model"`
		Effort string `json:"effort"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.chats.SetChatSelection(rctx, id, body.Model, body.Effort); err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteNoContent(ctx)
}

// Delete handles DELETE .../chats/:id: hard-deletes the chat and every thread below it. Unlike a
// folder delete, threads are not promoted: a thread continues its parent's turns and would be left
// without a premise. Only the tree usecase knows which chats those are.
func (h *Handlers) Delete(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	if _, ok := h.requireChatInWorkspace(ctx, id); !ok {
		return
	}

	removed, err := h.folders.DeleteChat(rctx, id, libs.DeleteConsentOf(ctx))
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	taken := removed.Taken
	if taken == nil {
		taken = []string{}
	}
	libs.WriteQueryOK(ctx, chatDeleted{TakenChatIDs: taken})
}

// chatDeleted names the chats a delete's git cascade took beyond the selection.
type chatDeleted struct {
	TakenChatIDs []string `json:"takenChatIds"`
}
