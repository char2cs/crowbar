package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Create handles POST /v0/workspaces/:wsId/chats.
func (h *Handlers) Create(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	var body struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	chat, err := h.chatUsecase.CreateChat(rctx, body.ID, wsID, body.Title, time.Now())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, chat)
}

// List handles GET /v0/workspaces/:wsId/chats.
func (h *Handlers) List(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	chats, err := h.chatRepo.ListByWorkspace(rctx, wsID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, chats)
}

// Fork handles POST /v0/chats/:id/fork.
func (h *Handlers) Fork(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	chat, err := h.chatUsecase.ForkChat(rctx, id, time.Now())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, chat)
}

// Rename handles PATCH /v0/chats/:id.
func (h *Handlers) Rename(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	var body struct {
		Title string `json:"title"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	chat, err := h.chatUsecase.RenameChat(rctx, id, body.Title)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, chat)
}

// Delete handles DELETE /v0/chats/:id.
func (h *Handlers) Delete(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	if err := h.chatUsecase.DeleteChat(rctx, id, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}
