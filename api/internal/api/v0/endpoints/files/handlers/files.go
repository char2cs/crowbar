package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Tree handles GET /v0/workspaces/:wsId/files
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

	ctx.JSON(http.StatusOK, nodes)
}

// ReadContent handles GET /v0/workspaces/:wsId/files/content
func (h *Handlers) ReadContent(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := ctx.Param("wsId")
	filePath := ctx.Query("path")

	if filePath == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	content, err := h.files.ReadContent(rctx, wsID, filePath)
	if err != nil {
		fileError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, content)
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
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if body.Path == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	if err := h.files.WriteContent(rctx, wsID, body.Path, body.Content, time.Now()); err != nil {
		fileError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
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
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if body.Path == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	now := time.Now()

	switch body.Type {
	case "dir":
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

	ctx.Status(http.StatusCreated)
}

// Rename handles PATCH /v0/workspaces/:wsId/files
func (h *Handlers) Rename(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := ctx.Param("wsId")

	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if body.From == "" || body.To == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "from and to are required"})
		return
	}

	if err := h.files.Rename(rctx, wsID, body.From, body.To, time.Now()); err != nil {
		fileError(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}

// Delete handles DELETE /v0/workspaces/:wsId/files
func (h *Handlers) Delete(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := ctx.Param("wsId")
	filePath := ctx.Query("path")

	if filePath == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	if err := h.files.Delete(rctx, wsID, filePath, time.Now()); err != nil {
		fileError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// fileError maps file usecase errors to HTTP responses.
func fileError(
	ctx *gin.Context,
	err error,
) {
	msg := err.Error()
	if strings.Contains(msg, "not found") || strings.Contains(msg, "no such file") {
		ctx.JSON(http.StatusNotFound, gin.H{"error": msg})
		return
	}
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": msg})
}
