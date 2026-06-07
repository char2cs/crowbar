// Package handlers holds the gin handlers backing the files endpoint: the lazy
// file tree, the read/save file-content pair, and the create/rename/delete
// filesystem mutations (02 §2.4).
//
// File mutations are keyed by their workspace-relative path rather than a UUID,
// so every mutation response echoes the affected path under the envelope's
// mutation id shape (data: { "id": "<path>" }) via libs.WriteMutationOK.
//
// The tree handler passes a nil git-status provider: tree decorations are an
// optional concern carried by the file-status WebSocket stream, and the fs
// engine's tree walker tolerates a nil provider by emitting undecorated nodes.
package handlers

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Files is the file usecase surface the handlers need: the lazy tree read, the
// read/write content pair, and the create-file, create-dir, rename, and delete
// mutations. It mirrors file.Usecase so the live usecase satisfies it directly.
type Files interface {
	Tree(
		ctx context.Context,
		wsID string,
		dirPath string,
		provider file.FileStatusProvider,
	) ([]domain.FileNode, error)
	ReadContent(
		ctx context.Context,
		wsID string,
		filePath string,
	) (domain.FileContent, error)
	WriteContent(
		ctx context.Context,
		wsID string,
		filePath string,
		content string,
		now time.Time,
	) error
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

// Handlers serves the /v0 file routes from the file usecase. now supplies the
// timestamp for the working-tree resync that follows each mutation.
type Handlers struct {
	files Files
	now   func() time.Time
}

// New builds the files Handlers from the file usecase and a clock.
func New(
	files Files,
	now func() time.Time,
) *Handlers {
	return &Handlers{
		files: files,
		now:   now,
	}
}
