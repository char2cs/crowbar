package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type Dismisser interface {
	Dismiss(ctx context.Context, id string) (domain.Operation, error)
}

type Handlers struct {
	ops Dismisser
}

func New(ops Dismisser) *Handlers {
	return &Handlers{ops: ops}
}

func (h *Handlers) Dismiss(c *gin.Context) {
	op, err := h.ops.Dismiss(c.Request.Context(), c.Param("id"))
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteQueryWithStatus(c, http.StatusOK, op)
}
