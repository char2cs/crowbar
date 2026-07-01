// Package chats mounts the v0 chat lifecycle REST and WebSocket routes.
package chats

import (
	"github.com/gin-gonic/gin"

	chathandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chats/handlers"
)

// Register mounts the chat lifecycle REST routes and WebSocket upgrade routes
// on the supplied router group.
func Register(
	rg *gin.RouterGroup,
	chatUsecase chathandlers.ChatUsecase,
	chatRepo chathandlers.ChatRepo,
	wsReader chathandlers.WorkspaceReader,
	chatsWS gin.HandlerFunc,
	chatStreamWS gin.HandlerFunc,
) {
	h := chathandlers.New(chatUsecase, chatRepo, wsReader)

	rg.POST("/workspaces/:wsId/chats", h.Create)
	rg.GET("/workspaces/:wsId/chats", h.List)
	rg.POST("/chats/:id/fork", h.Fork)
	rg.PATCH("/chats/:id", h.Rename)
	rg.DELETE("/chats/:id", h.Delete)
	rg.GET("/ws/chats", chatsWS)
	rg.GET("/ws/chats/:chatId/stream", chatStreamWS)
}
