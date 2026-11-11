// Package handlers holds the gin handlers backing the files endpoint: tree
// listing, create, copy, rename, and delete.
package handlers

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	fileusecase "github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Files is the file usecase surface the handlers need.
type Files interface {
	// Tree returns one level of the file tree for a workspace.
	Tree(
		ctx context.Context,
		wsID string,
		dirPath string,
		provider fileusecase.FileStatusProvider,
	) ([]domain.FileNode, error)

	// CreateFile creates a file and resyncs the working tree.
	CreateFile(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error

	// CreateDir creates a directory and resyncs the working tree.
	CreateDir(
		ctx context.Context,
		wsID string,
		dirPath string,
		now time.Time,
	) error

	// Copy duplicates a file or directory byte-faithfully and resyncs the
	// working tree.
	Copy(
		ctx context.Context,
		wsID string,
		sourcePath string,
		destPath string,
		now time.Time,
	) error

	// Rename renames a path and resyncs the working tree.
	Rename(
		ctx context.Context,
		wsID string,
		oldPath string,
		newPath string,
		now time.Time,
	) error

	// Delete removes a path and resyncs the working tree.
	Delete(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error
}

// Handlers serves the files routes from the file usecase, on both scoping
// groups they are mounted at (routes.go).
type Handlers struct {
	files Files
}

// New builds the files Handlers from the file usecase.
func New(files Files) *Handlers {
	return &Handlers{files: files}
}

// workspaceID answers which worktree this request acts on. The worktree that
// resolveChatWorktree stashed in reqscope wins over a :wsId param, so a mount
// carrying both can never act on the URL instead of on the chat.
func (h *Handlers) workspaceID(
	ctx *gin.Context,
) string {
	if ws, ok := reqscope.Workspace(ctx); ok {
		return ws.ID
	}
	return ctx.Param("wsId")
}
