package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Status handles GET /v0/workspaces/:wsId/git/status, returning the workspace's
// full working-tree status as a GitStatusDTO. The route is dual-served: a
// WebSocket upgrade is routed to the live git stream before this handler runs.
func (h *Handlers) Status(
	c *gin.Context,
) {
	status, err := h.git.Status(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		code, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, code, msg)
		return
	}
	libs.WriteQueryOK(c, dto.GitStatusDTOFrom(status))
}
