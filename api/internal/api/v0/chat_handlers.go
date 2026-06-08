package v0

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// registerChatHandlers mounts the chat lifecycle REST routes on rg.
func registerChatHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.POST("/workspaces/:wsId/chats", c.handleChatCreate)
	rg.GET("/workspaces/:wsId/chats", c.handleChatList)
	rg.POST("/chats/:id/fork", c.handleChatFork)
	rg.PATCH("/chats/:id", c.handleChatRename)
	rg.DELETE("/chats/:id", c.handleChatDelete)
}

// handleChatCreate POST /v0/workspaces/:wsId/chats
func (c *Container) handleChatCreate(
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

	chat, err := c.app.Usecases.Chat.CreateChat(rctx, body.ID, wsID, body.Title, time.Now())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, chat)
}

// handleChatList GET /v0/workspaces/:wsId/chats
func (c *Container) handleChatList(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	wsID := ctx.Param("wsId")

	chats, err := c.app.Repositories.Chat.ListByWorkspace(rctx, wsID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, chats)
}

// handleChatFork POST /v0/chats/:id/fork
func (c *Container) handleChatFork(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	chat, err := c.app.Usecases.Chat.ForkChat(rctx, id, time.Now())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, chat)
}

// handleChatRename PATCH /v0/chats/:id
func (c *Container) handleChatRename(
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

	chat, err := c.app.Usecases.Chat.RenameChat(rctx, id, body.Title)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, chat)
}

// handleChatDelete DELETE /v0/chats/:id
func (c *Container) handleChatDelete(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()
	id := ctx.Param("id")

	if err := c.app.Usecases.Chat.DeleteChat(rctx, id, time.Now()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}
