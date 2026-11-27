package handlers

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	fileusecase "github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type Files interface {
	Tree(
		ctx context.Context,
		wsID string,
		dirPath string,
		provider fileusecase.FileStatusProvider,
	) ([]domain.FileNode, error)

	CreateFile(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error

	CreateDir(
		ctx context.Context,
		wsID string,
		dirPath string,
		now time.Time,
	) error

	Copy(
		ctx context.Context,
		wsID string,
		sourcePath string,
		destPath string,
		now time.Time,
	) error

	Rename(
		ctx context.Context,
		wsID string,
		oldPath string,
		newPath string,
		now time.Time,
	) error

	Delete(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error
}

type Handlers struct {
	files Files
}

func New(files Files) *Handlers {
	return &Handlers{files: files}
}

func (h *Handlers) workspaceID(
	ctx *gin.Context,
) string {
	if ws, ok := reqscope.Workspace(ctx); ok {
		return ws.ID
	}
	return ctx.Param("wsId")
}
