package v0

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/char2cs/crowbar/api/internal/fixtures"
)

type FlowsHandler struct{ store *fixtures.Store }

func NewFlowsHandler(store *fixtures.Store) *FlowsHandler {
	return &FlowsHandler{store: store}
}

func (h *FlowsHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.Flows)
}
