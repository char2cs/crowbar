package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Branches handles GET /v0/workspaces/:wsId/git/branches, returning the
// workspace's branches as a BranchDTO list.
func (h *Handlers) Branches(
	c *gin.Context,
) {
	branches, err := h.git.Branches(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		code, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, code, msg)
		return
	}
	libs.WriteQueryOK(c, dto.BranchDTOList(branches))
}
