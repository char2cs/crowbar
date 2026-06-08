// Package handlers holds the gin handlers backing the files endpoint: tree
// listing, content read/write, create, rename, and delete.
package handlers

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	fileusecase "github.com/char2cs/crowbar/api/internal/app/usecases/file"
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

	// ReadContent reads a file in a workspace.
	ReadContent(
		ctx context.Context,
		wsID string,
		filePath string,
	) (domain.FileContent, error)

	// WriteContent writes a file and resyncs the working tree.
	WriteContent(
		ctx context.Context,
		wsID string,
		filePath string,
		content string,
		now time.Time,
	) error

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

// Handlers serves the /v0/workspaces/:wsId/files routes from the file usecase.
type Handlers struct {
	files Files
}

// New builds the files Handlers from the file usecase.
func New(files Files) *Handlers {
	return &Handlers{files: files}
}
