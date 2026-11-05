package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// FileTree handles GET /v0/projects/:projectId/home/files/tree.
func (h *Handlers) FileTree(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	dirPath := c.DefaultQuery("path", ".")
	nodes, err := h.files.Tree(c.Request.Context(), ws.ID, dirPath, nil)
	if err != nil {
		libs.Fail(c, err)
		return
	}
	if nodes == nil {
		nodes = []domain.FileNode{}
	}
	libs.WriteQueryOK(c, nodes)
}

// CreateFile handles POST /v0/projects/:projectId/home/files.
func (h *Handlers) CreateFile(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	var body struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Path == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path is required")
		return
	}
	now := time.Now()
	switch body.Type {
	case "dir", "directory":
		if err := h.files.CreateDir(c.Request.Context(), ws.ID, body.Path, now); err != nil {
			libs.Fail(c, err)
			return
		}
	default:
		if err := h.files.CreateFile(c.Request.Context(), ws.ID, body.Path, now); err != nil {
			libs.Fail(c, err)
			return
		}
	}
	libs.WriteMutationOK(c, http.StatusCreated, body.Path)
}

// CopyFile handles POST /v0/projects/:projectId/home/files/copy, duplicating
// sourcePath to destPath server-side so binaries never round-trip through a text write.
func (h *Handlers) CopyFile(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	var body struct {
		SourcePath string `json:"sourcePath"`
		DestPath   string `json:"destPath"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.SourcePath == "" || body.DestPath == "" {
		libs.WriteErr(c, http.StatusBadRequest, "sourcePath and destPath are required")
		return
	}
	if err := h.files.Copy(c.Request.Context(), ws.ID, body.SourcePath, body.DestPath, time.Now()); err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteMutationOK(c, http.StatusCreated, body.DestPath)
}

// RenameFile handles PATCH /v0/projects/:projectId/home/files.
func (h *Handlers) RenameFile(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	// {path, newPath} is the rename contract shared with the chat-scoped files
	// handler and the web client.
	var body struct {
		Path    string `json:"path"`
		NewPath string `json:"newPath"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Path == "" || body.NewPath == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path and newPath are required")
		return
	}
	if err := h.files.Rename(c.Request.Context(), ws.ID, body.Path, body.NewPath, time.Now()); err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, body.NewPath)
}

// DeleteFile handles DELETE /v0/projects/:projectId/home/files.
func (h *Handlers) DeleteFile(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Path == "" {
		body.Path = c.Query("path")
	}
	if body.Path == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path is required")
		return
	}
	if err := h.files.Delete(c.Request.Context(), ws.ID, body.Path, time.Now()); err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, body.Path)
}
