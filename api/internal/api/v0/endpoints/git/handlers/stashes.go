package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Stashes handles GET /v0/workspaces/:wsId/git/stashes, returning the
// workspace's stash entries as a StashDTO list.
func (h *Handlers) Stashes(
	c *gin.Context,
) {
	stashes, err := h.git.Stashes(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		code, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, code, msg)
		return
	}
	libs.WriteQueryOK(c, dto.StashDTOList(stashes))
}
