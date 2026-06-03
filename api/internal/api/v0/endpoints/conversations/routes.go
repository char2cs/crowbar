package conversations

import (
	"github.com/gin-gonic/gin"

	convhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/conversations/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Register mounts all conversation routes on rg.
func Register(
	rg *gin.RouterGroup,
	svc usecases.ConversationUsecase,
) {
	h := convhandlers.New(svc)
	rg.GET("/tasks/:id/messages", h.List)
}
