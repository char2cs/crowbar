package tasks

import (
	"github.com/gin-gonic/gin"

	taskhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/tasks/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Register mounts all task routes on rg.
func Register(
	rg *gin.RouterGroup,
	svc usecases.TaskUsecase,
	wsTask gin.HandlerFunc,
) {
	h := taskhandlers.New(svc)
	rg.POST("/repositories/:id/tasks", h.Create)
	rg.GET("/repositories/:id/tasks", h.List)
	rg.GET("/tasks/:id", ws.Dispatch(h.Get, wsTask))
	rg.POST("/tasks/:id/archive", h.Archive)
	rg.POST("/tasks/:id/pause", h.Pause)
	rg.POST("/tasks/:id/resume", h.Resume)
	rg.POST("/tasks/:id/retry", h.Retry)
	rg.POST("/tasks/:id/force-transition", h.ForceTransition)
}
