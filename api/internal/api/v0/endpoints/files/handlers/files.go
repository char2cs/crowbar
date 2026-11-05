package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Tree handles GET /v0/chats/:chatId/files/tree
func (h *Handlers) Tree(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := h.workspaceID(ctx)
	dirPath := ctx.DefaultQuery("path", ".")

	nodes, err := h.files.Tree(rctx, wsID, dirPath, nil)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}

	if nodes == nil {
		nodes = []domain.FileNode{}
	}

	libs.WriteQueryOK(ctx, nodes)
}

// Create handles POST /v0/chats/:chatId/files
func (h *Handlers) Create(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := h.workspaceID(ctx)

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
			libs.Fail(ctx, err)
			return
		}
	default:
		if err := h.files.CreateFile(rctx, wsID, body.Path, now); err != nil {
			libs.Fail(ctx, err)
			return
		}
	}

	libs.WriteMutationOK(ctx, http.StatusCreated, body.Path)
}

// Copy handles POST /v0/chats/:chatId/files/copy, duplicating sourcePath to
// destPath server-side so binary files never round-trip through a text write.
// Directories copy recursively; an existing destination is a 409.
func (h *Handlers) Copy(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := h.workspaceID(ctx)

	var body struct {
		SourcePath string `json:"sourcePath"`
		DestPath   string `json:"destPath"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}

	if body.SourcePath == "" || body.DestPath == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "sourcePath and destPath are required")
		return
	}

	if err := h.files.Copy(rctx, wsID, body.SourcePath, body.DestPath, time.Now()); err != nil {
		libs.Fail(ctx, err)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusCreated, body.DestPath)
}

// Rename handles PATCH /v0/chats/:chatId/files
func (h *Handlers) Rename(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := h.workspaceID(ctx)

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
		libs.Fail(ctx, err)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusOK, body.NewPath)
}

// Delete handles DELETE /v0/chats/:chatId/files
func (h *Handlers) Delete(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := h.workspaceID(ctx)

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
		libs.Fail(ctx, err)
		return
	}

	libs.WriteMutationOK(ctx, http.StatusOK, body.Path)
}
