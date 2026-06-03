package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Handlers holds conversation HTTP handlers.
type Handlers struct {
	svc usecases.ConversationUsecase
}

// New returns a Handlers wired to svc.
func New(
	svc usecases.ConversationUsecase,
) *Handlers {
	return &Handlers{svc: svc}
}

// List handles GET /tasks/:id/messages.
func (h *Handlers) List(
	c *gin.Context,
) {
	taskID := c.Param("id")
	messages, err := h.svc.List(c.Request.Context(), taskID)
	if err != nil {
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), taskID)
		return
	}
	libs.WriteQueryOK(c, messages)
}
