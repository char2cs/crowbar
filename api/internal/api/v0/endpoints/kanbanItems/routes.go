package kanbanItems

import (
	"github.com/gin-gonic/gin"

	kanbanhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/kanbanItems/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Register mounts all kanban-item routes on rg.
func Register(
	rg *gin.RouterGroup,
	svc usecases.KanbanItemUsecase,
	wsKanban gin.HandlerFunc,
) {
	h := kanbanhandlers.New(svc)
	rg.GET("/tasks/:id/kanban-items", ws.Dispatch(h.List, wsKanban))
}
