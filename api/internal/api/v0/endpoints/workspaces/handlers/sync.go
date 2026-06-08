package handlers

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Sync handles POST /v0/workspaces/:wsId/sync, recomputing the working-tree
// summary from git and persisting the updated workspace state.
func (h *Handlers) Sync(
	c *gin.Context,
) {
	id := c.Param("wsId")
	ws, err := h.reader.SyncWorkingTreeState(c.Request.Context(), id, time.Now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.WorkspaceDTOFrom(ws))
}
