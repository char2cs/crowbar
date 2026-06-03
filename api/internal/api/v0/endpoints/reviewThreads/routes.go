package reviewThreads

import (
	"github.com/gin-gonic/gin"

	threadhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/reviewThreads/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Register mounts all review-thread routes on rg.
func Register(
	rg *gin.RouterGroup,
	svc usecases.ReviewThreadUsecase,
	wsThreads gin.HandlerFunc,
) {
	h := threadhandlers.New(svc)
	rg.GET("/tasks/:id/threads", ws.Dispatch(h.List, wsThreads))
	rg.POST("/tasks/:id/threads", h.Open)
	rg.POST("/threads/:id/messages", h.PostMessage)
	rg.POST("/threads/:id/force-approve", h.ForceApprove)
}
