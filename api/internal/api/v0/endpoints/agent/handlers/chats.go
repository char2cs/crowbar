package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Create handles POST /v0/agent/chats: spawns a fresh AgentChat and its first
// AgentSegment, launching the provider's vendor CLI in a PTY. It responds with
// the new chat's id; the spawned segment id is not surfaced here (the client
// reads it back via GET /v0/agent/chats/:id).
func (h *Handlers) Create(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	var body struct {
		WorkspaceID string `json:"workspaceId"`
		Provider    string `json:"provider"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	chatID, _, err := h.usecase.SpawnChat(rctx, body.WorkspaceID, body.Provider)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusCreated, chatID)
}

// List handles GET /v0/agent/chats.
func (h *Handlers) List(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	chats, err := h.usecase.ListChats(rctx)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteQueryOK(ctx, dto.AgentChatDTOList(chats))
}

// Get handles GET /v0/agent/chats/:id, returning the chat plus its ordered
// segment history.
func (h *Handlers) Get(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	chat, err := h.usecase.GetChat(rctx, id)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	segs, err := h.usecase.SegmentsFor(rctx, id)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}

	libs.WriteQueryOK(ctx, dto.AgentChatDetailDTOFrom(chat, segs))
}

// Rename handles POST /v0/agent/chats/:id/rename: sets the chat's title.
// `?source=agent` applies the agent precedence rule (skip if user-locked); the
// default (a human/FE rename) sets unconditionally and locks.
func (h *Handlers) Rename(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")
	source := ctx.Query("source")

	var body struct {
		Title string `json:"title"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.usecase.RenameChat(rctx, id, body.Title, source); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, msg)
		return
	}
	libs.WriteAccepted(ctx)
}
