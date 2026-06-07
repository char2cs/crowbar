package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// createRequest is the POST /v0/workspaces/:wsId/files body: the
// workspace-relative path to create and whether it is a file or a directory.
// type accepts "file", "folder", or "directory".
type createRequest struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

// renameRequest is the PATCH /v0/workspaces/:wsId/files body: the existing path
// and the destination path it is renamed or moved to.
type renameRequest struct {
	Path    string `json:"path"`
	NewPath string `json:"newPath"`
}

// deleteRequest is the DELETE /v0/workspaces/:wsId/files body: the
// workspace-relative path to remove.
type deleteRequest struct {
	Path string `json:"path"`
}

// Create handles POST /v0/workspaces/:wsId/files, creating a file or directory
// at the body's path and resyncing the working tree. The path and type are
// required; an unknown type or a missing field yields 400. The response echoes
// the created path with status 201.
func (h *Handlers) Create(
	c *gin.Context,
) {
	var body createRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Path == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path is required")
		return
	}
	if body.Type != "file" && !isDirType(body.Type) {
		libs.WriteErr(c, http.StatusBadRequest, "type must be file, folder, or directory")
		return
	}
	err := h.create(c, body)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusCreated, body.Path)
}

func (h *Handlers) create(
	c *gin.Context,
	body createRequest,
) error {
	if isDirType(body.Type) {
		return h.files.CreateDir(
			c.Request.Context(),
			c.Param("wsId"),
			body.Path,
			h.now(),
		)
	}
	return h.files.CreateFile(
		c.Request.Context(),
		c.Param("wsId"),
		body.Path,
		h.now(),
	)
}

// Rename handles PATCH /v0/workspaces/:wsId/files, renaming or moving path to
// newPath and resyncing the working tree. Both fields are required; a missing
// field yields 400. The response echoes the destination path.
func (h *Handlers) Rename(
	c *gin.Context,
) {
	var body renameRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Path == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path is required")
		return
	}
	if body.NewPath == "" {
		libs.WriteErr(c, http.StatusBadRequest, "newPath is required")
		return
	}
	err := h.files.Rename(
		c.Request.Context(),
		c.Param("wsId"),
		body.Path,
		body.NewPath,
		h.now(),
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, body.NewPath)
}

// Delete handles DELETE /v0/workspaces/:wsId/files, removing the body's path and
// resyncing the working tree. The path is required; a missing path yields 400.
// The response echoes the deleted path.
func (h *Handlers) Delete(
	c *gin.Context,
) {
	var body deleteRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Path == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path is required")
		return
	}
	err := h.files.Delete(
		c.Request.Context(),
		c.Param("wsId"),
		body.Path,
		h.now(),
	)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, body.Path)
}

func isDirType(
	t string,
) bool {
	return t == "folder" || t == "directory"
}
