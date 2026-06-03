package chat

import (
	"context"

	"github.com/gin-gonic/gin"

	appHub "github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/internal/handler"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agent"
)

// Re-export ChatFrame types so callers import from this package.
type ChatFrameType = agent.ChatFrameType
type ChatFrame = agent.ChatFrame

// ChatHandler handles a single WebSocket connection for a task chat session.
type ChatHandler interface {
	Handle(ctx context.Context, c *gin.Context, taskID string)
}

// ConversationWriter is the subset of the conversation repository used by the
// chat handler. Defined here to avoid an import cycle with the repositories
// package.
type ConversationWriter interface {
	Create(ctx context.Context, taskID string, role domain.ConversationMessageRole, msgType domain.ConversationMessageType, content string) (domain.ConversationMessage, error)
}

// NewHandler constructs a ChatHandler backed by the provided hub and conversation
// writer. The concrete implementation lives in chat/internal/handler.
func NewHandler(h appHub.ChatHub, conv ConversationWriter) ChatHandler {
	return handler.New(h, conv)
}

// Register mounts the chat WebSocket route on rg.
func Register(rg *gin.RouterGroup, h ChatHandler) {
	rg.GET("/tasks/:id/chat", func(c *gin.Context) {
		h.Handle(context.Background(), c, c.Param("id"))
	})
}
