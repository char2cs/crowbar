package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Blame handles GET /v0/workspaces/:wsId/blame, annotating each line of the
// ?path= file with its last-changing commit. The data field carries a
// BlameEntry array, empty for an unblamed file.
func (h *Handlers) Blame(
	c *gin.Context,
) {
	if !h.requireGit(c) {
		return
	}
	worktreePath, ok := h.worktreePath(c)
	if !ok {
		return
	}
	path := c.Query("path")
	if path == "" {
		libs.WriteErr(
			c,
			http.StatusBadRequest,
			"path is required",
		)
		return
	}
	entries, err := h.git.Blame(
		c.Request.Context(),
		worktreePath,
		path,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.BlameEntryDTOList(entries))
}
