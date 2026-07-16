package handlers

import (
	"net/http"
	"strings"
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
		homeFileError(c, err)
		return
	}
	if nodes == nil {
		nodes = []domain.FileNode{}
	}
	libs.WriteQueryOK(c, nodes)
}

// FileContent handles GET /v0/projects/:projectId/home/files/content.
func (h *Handlers) FileContent(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	filePath := c.Query("path")
	if filePath == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path is required")
		return
	}
	content, err := h.files.ReadContent(c.Request.Context(), ws.ID, filePath)
	if err != nil {
		homeFileError(c, err)
		return
	}
	libs.WriteQueryOK(c, content)
}

// SaveFileContent handles PUT /v0/projects/:projectId/home/files/content.
func (h *Handlers) SaveFileContent(c *gin.Context) {
	ws, ok := h.resolveHome(c)
	if !ok {
		return
	}
	var body struct {
		Path     string `json:"path"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Path == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path is required")
		return
	}
	if err := h.files.WriteContent(c.Request.Context(), ws.ID, body.Path, body.Content, body.Encoding, time.Now()); err != nil {
		homeFileError(c, err)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, body.Path)
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
			homeFileError(c, err)
			return
		}
	default:
		if err := h.files.CreateFile(c.Request.Context(), ws.ID, body.Path, now); err != nil {
			homeFileError(c, err)
			return
		}
	}
	libs.WriteMutationOK(c, http.StatusCreated, body.Path)
}

// CopyFile handles POST /v0/projects/:projectId/home/files/copy. It mirrors the
// workspace-scoped copy verb (files/handlers/files.go's Copy): sourcePath is
// duplicated to destPath (both workspace-relative) byte-faithfully, so Duplicate
// in the home explorer never corrupts binaries via the base64 content round trip.
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
		homeFileError(c, err)
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
	// {path, newPath} — the shared rename contract the workspace files handler
	// (files/handlers/files.go Rename) and the FE (file-tree-api renameFileNode /
	// platform moveFile) already use. Binding {oldPath, newPath} here made every
	// home-project explorer rename 400 on the shape mismatch.
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
		homeFileError(c, err)
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
		homeFileError(c, err)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, body.Path)
}

// homeFileError maps file usecase errors to HTTP responses.
func homeFileError(c *gin.Context, err error) {
	status, msg := libs.StatusAndMessage(err)
	if status == http.StatusInternalServerError &&
		(strings.Contains(msg, "not found") || strings.Contains(msg, "no such file")) {
		status = http.StatusNotFound
	}
	libs.WriteErr(c, status, msg)
}
