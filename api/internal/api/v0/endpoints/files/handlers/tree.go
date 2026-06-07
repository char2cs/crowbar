package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// Tree handles GET /v0/workspaces/:wsId/files/tree, returning one level of the
// workspace file tree as a FileNodeDTO[]. The optional ?path= query selects a
// subtree for lazy expansion; an absent path yields the workspace root.
func (h *Handlers) Tree(
	c *gin.Context,
) {
	nodes, err := h.files.Tree(
		c.Request.Context(),
		c.Param("wsId"),
		c.Query("path"),
		nil,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	dtos := dto.FileNodeDTOList(nodes)
	if dtos == nil {
		dtos = []dto.FileNodeDTO{}
	}
	libs.WriteQueryOK(c, dtos)
}
