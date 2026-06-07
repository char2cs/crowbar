package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// ProtectedBranches handles GET /v0/repos/:id/protected-branches, resolving the
// repo root from the workspace whose id is :id and returning its protected
// branch names. A disabled capability still yields a 200 envelope carrying the
// default protected branches. The data field is an array, empty for a repo with
// no protected branches.
func (h *Handlers) ProtectedBranches(
	c *gin.Context,
) {
	if !h.requireProvider(c) {
		return
	}
	ws, ok := h.workspace(c, "id")
	if !ok {
		return
	}
	branches, err := h.provider.ProtectedBranches(
		c.Request.Context(),
		ws.WorktreePath,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.ProtectedBranchList(branches))
}
