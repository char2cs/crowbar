package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

// saveRequest is the PUT /v0/workspaces/:wsId/files/content body: the
// workspace-relative path to write and the full replacement content.
type saveRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ReadContent handles GET /v0/workspaces/:wsId/files/content, reading the file
// named by the required ?path= query and returning its content and encoding. A
// missing path yields 400.
func (h *Handlers) ReadContent(
	c *gin.Context,
) {
	path := c.Query("path")
	if path == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path is required")
		return
	}
	content, err := h.files.ReadContent(
		c.Request.Context(),
		c.Param("wsId"),
		path,
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.FileContentDTOFrom(content))
}

// SaveContent handles PUT /v0/workspaces/:wsId/files/content, writing the body's
// content to the named path and resyncing the working tree. The path is
// required; a missing or empty path yields 400. The response echoes the path.
func (h *Handlers) SaveContent(
	c *gin.Context,
) {
	var body saveRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Path == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path is required")
		return
	}
	err := h.files.WriteContent(
		c.Request.Context(),
		c.Param("wsId"),
		body.Path,
		body.Content,
		h.now(),
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, body.Path)
}
