package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// OperationContinue handles POST /v0/workspaces/:wsId/git/operation/continue,
// finalizing the in-progress merge, rebase, or pull. The response echoes the
// workspace id.
func (h *Handlers) OperationContinue(
	c *gin.Context,
) {
	err := h.git.OperationContinue(c.Request.Context(), c.Param("wsId"), h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}

// OperationAbort handles POST /v0/workspaces/:wsId/git/operation/abort, rolling
// back the in-progress merge, rebase, or pull. The response echoes the
// workspace id.
func (h *Handlers) OperationAbort(
	c *gin.Context,
) {
	err := h.git.OperationAbort(c.Request.Context(), c.Param("wsId"), h.now())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("wsId"))
}
