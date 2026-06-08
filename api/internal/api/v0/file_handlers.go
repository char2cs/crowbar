package v0

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// registerFileHandlers mounts the file REST routes on rg.
func registerFileHandlers(
	rg *gin.RouterGroup,
	c *Container,
) {
	rg.GET("/workspaces/:wsId/files/content", c.handleFileRead)
	rg.GET("/workspaces/:wsId/files", c.handleFileTree)
	rg.PUT("/workspaces/:wsId/files/content", c.handleFileWrite)
	rg.POST("/workspaces/:wsId/files", c.handleFileCreate)
	rg.PATCH("/workspaces/:wsId/files", c.handleFileRename)
	rg.DELETE("/workspaces/:wsId/files", c.handleFileDelete)
}

// handleFileTree GET /v0/workspaces/:wsId/files
func (c *Container) handleFileTree(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := ctx.Param("wsId")
	dirPath := ctx.DefaultQuery("path", ".")

	nodes, err := c.app.Usecases.File.Tree(rctx, wsID, dirPath, nil)
	if err != nil {
		fileError(ctx, err)
		return
	}

	if nodes == nil {
		nodes = []domain.FileNode{}
	}

	ctx.JSON(http.StatusOK, nodes)
}

// handleFileRead GET /v0/workspaces/:wsId/files/content
func (c *Container) handleFileRead(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := ctx.Param("wsId")
	filePath := ctx.Query("path")

	if filePath == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	content, err := c.app.Usecases.File.ReadContent(rctx, wsID, filePath)
	if err != nil {
		fileError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, content)
}

// handleFileWrite PUT /v0/workspaces/:wsId/files/content
func (c *Container) handleFileWrite(
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

	if err := c.app.Usecases.File.WriteContent(rctx, wsID, body.Path, body.Content, time.Now()); err != nil {
		fileError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// handleFileCreate POST /v0/workspaces/:wsId/files
func (c *Container) handleFileCreate(
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
		if err := c.app.Usecases.File.CreateDir(rctx, wsID, body.Path, now); err != nil {
			fileError(ctx, err)
			return
		}
	default:
		if err := c.app.Usecases.File.CreateFile(rctx, wsID, body.Path, now); err != nil {
			fileError(ctx, err)
			return
		}
	}

	ctx.Status(http.StatusCreated)
}

// handleFileRename PATCH /v0/workspaces/:wsId/files
func (c *Container) handleFileRename(
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

	if err := c.app.Usecases.File.Rename(rctx, wsID, body.From, body.To, time.Now()); err != nil {
		fileError(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}

// handleFileDelete DELETE /v0/workspaces/:wsId/files
func (c *Container) handleFileDelete(
	ctx *gin.Context,
) {
	rctx := ctx.Request.Context()

	wsID := ctx.Param("wsId")
	filePath := ctx.Query("path")

	if filePath == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	if err := c.app.Usecases.File.Delete(rctx, wsID, filePath, time.Now()); err != nil {
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
