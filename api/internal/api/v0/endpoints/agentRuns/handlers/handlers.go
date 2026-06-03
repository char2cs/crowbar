package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Handlers holds agent-run HTTP handlers.
type Handlers struct {
	svc usecases.AgentRunUsecase
}

// New returns a Handlers wired to svc.
func New(
	svc usecases.AgentRunUsecase,
) *Handlers {
	return &Handlers{svc: svc}
}

// List handles GET /tasks/:id/agent-runs.
func (h *Handlers) List(
	c *gin.Context,
) {
	taskID := c.Param("id")
	runs, err := h.svc.List(c.Request.Context(), taskID)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), taskID)
		return
	}
	libs.WriteQueryOK(c, runs)
}

// Interrupt handles POST /agent-runs/:id/interrupt.
func (h *Handlers) Interrupt(
	c *gin.Context,
) {
	id := c.Param("id")
	if err := h.svc.Interrupt(c.Request.Context(), id); err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", id)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	c.Status(http.StatusNoContent)
}
