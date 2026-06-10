package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Tree handles GET /v0/workspaces/:wsId/files/tree
func (h *Handlers) Tree(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := ctx.Param("wsId")
	dirPath := ctx.DefaultQuery("path", ".")

	nodes, err := h.files.Tree(rctx, wsID, dirPath, nil)
	if err != nil {
		fileError(ctx, err)
		return
	}

	if nodes == nil {
		nodes = []domain.FileNode{}
	}

	libs.WriteQueryOK(ctx, nodes)
}

// ReadContent handles GET /v0/workspaces/:wsId/files/content
func (h *Handlers) ReadContent(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := ctx.Param("wsId")
	filePath := ctx.Query("path")

	if filePath == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path is required")
		return
	}

	content, err := h.files.ReadContent(rctx, wsID, filePath)
	if err != nil {
		fileError(ctx, err)
		return
	}

	libs.WriteQueryOK(ctx, content)
}

// SaveContent handles PUT /v0/workspaces/:wsId/files/content
func (h *Handlers) SaveContent(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := ctx.Param("wsId")

	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if body.Path == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path is required")
		return
	}

	if err := h.files.WriteContent(rctx, wsID, body.Path, body.Content, time.Now()); err != nil {
		fileError(ctx, err)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusOK, body.Path)
}

// Create handles POST /v0/workspaces/:wsId/files
func (h *Handlers) Create(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := ctx.Param("wsId")

	var body struct {
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if body.Path == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path is required")
		return
	}

	now := time.Now()

	switch body.Type {
	case "dir", "directory":
		if err := h.files.CreateDir(rctx, wsID, body.Path, now); err != nil {
			fileError(ctx, err)
			return
		}
	default:
		if err := h.files.CreateFile(rctx, wsID, body.Path, now); err != nil {
			fileError(ctx, err)
			return
		}
	}

	libs.WriteMutationOK(ctx, http.StatusCreated, body.Path)
}

// Rename handles PATCH /v0/workspaces/:wsId/files
func (h *Handlers) Rename(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := ctx.Param("wsId")

	var body struct {
		Path    string `json:"path"`
		NewPath string `json:"newPath"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if body.Path == "" || body.NewPath == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path and newPath are required")
		return
	}

	if err := h.files.Rename(rctx, wsID, body.Path, body.NewPath, time.Now()); err != nil {
		fileError(ctx, err)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusOK, body.NewPath)
}

// Delete handles DELETE /v0/workspaces/:wsId/files
func (h *Handlers) Delete(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := ctx.Param("wsId")

	var body struct {
		Path string `json:"path"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil || body.Path == "" {
		body.Path = ctx.Query("path")
	}

	if body.Path == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path is required")
		return
	}

	if err := h.files.Delete(rctx, wsID, body.Path, time.Now()); err != nil {
		fileError(ctx, err)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusOK, body.Path)
}

// fileError maps file usecase errors to HTTP responses. Typed sentinels (e.g.
// apperr.ErrLocked -> 409, fs.ErrNotExist -> 404) go through the shared
// libs.StatusAndMessage mapping; untyped not-found errors fall back to a
// message sniff for 404.
func fileError(
	ctx *gin.Context,
	err error,
) {
	status, msg := libs.StatusAndMessage(err)
	if status == http.StatusInternalServerError &&
		(strings.Contains(msg, "not found") || strings.Contains(msg, "no such file")) {
		status = http.StatusNotFound
	}
	libs.WriteErr(ctx, status, msg)
}
