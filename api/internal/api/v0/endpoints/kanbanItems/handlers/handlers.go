package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Handlers holds kanban-item HTTP handlers.
type Handlers struct {
	svc usecases.KanbanItemUsecase
}

// New returns a Handlers wired to svc.
func New(
	svc usecases.KanbanItemUsecase,
) *Handlers {
	return &Handlers{svc: svc}
}

// List handles GET /tasks/:id/kanban-items.
func (h *Handlers) List(
	c *gin.Context,
) {
	taskID := c.Param("id")
	items, err := h.svc.List(c.Request.Context(), taskID)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), taskID)
		return
	}
	libs.WriteQueryOK(c, items)
}
