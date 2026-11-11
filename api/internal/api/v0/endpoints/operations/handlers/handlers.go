// Package handlers holds the HTTP handlers of the operations routes.
package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Dismisser marks a settled operation dismissed.
type Dismisser interface {
	Dismiss(ctx context.Context, id string) (domain.Operation, error)
}

// Handlers serves the operations routes.
type Handlers struct {
	ops Dismisser
}

// New builds the handlers over the operation service.
func New(ops Dismisser) *Handlers {
	return &Handlers{ops: ops}
}

// Dismiss DELETE /v0/operations/:id answers the dismissed operation.
func (h *Handlers) Dismiss(c *gin.Context) {
	op, err := h.ops.Dismiss(c.Request.Context(), c.Param("id"))
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteQueryWithStatus(c, http.StatusOK, op)
}
