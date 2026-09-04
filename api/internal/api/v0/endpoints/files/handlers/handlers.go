// Package handlers holds the gin handlers backing the files endpoint: tree
// listing, content read/write, create, rename, and delete.
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

	// ReadContent reads a file in a workspace.
	ReadContent(
		ctx context.Context,
		wsID string,
		filePath string,
	) (domain.FileContent, error)

	// WriteContent writes a file and resyncs the working tree. encoding is
	// "base64" for a byte-faithful binary payload or "" / "utf8" for raw UTF-8.
	WriteContent(
		ctx context.Context,
		wsID string,
		filePath string,
		content string,
		encoding string,
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

// workspaceID answers which worktree this request acts on, for either of the
// two groups files is currently mounted on (routes.go): the chat-scoped
// mount and the home mount (endpoints/home), whose RequireHomeWorkspace
// injects the resolved home workspace as the :wsId param.
//
// On /v0/chats/:chatId/files/... the chat group's resolveChatWorktree
// middleware has already resolved the chat's worktree and stashed the
// workspace on the context, so the answer is read back from reqscope — never
// resolved a second time per request, and never taken from a URL, because no
// chat-scoped URL carries a workspace id to take it from (spec law 1). The
// old /projects/:projectId/repos/:repoId/workspaces/:wsId/files/... mount is
// gone (spec §8 step 6); the :wsId branch below now serves the home mount
// alone.
//
// reqscope is consulted FIRST because it is the resolved truth: the mounts are
// disjoint, so exactly one source is ever populated, and preferring the
// middleware's answer means a future mount that carries both cannot silently
// act on the URL instead of on the chat.
func (h *Handlers) workspaceID(
	ctx *gin.Context,
) string {
	if ws, ok := reqscope.Workspace(ctx); ok {
		return ws.ID
	}
	return ctx.Param("wsId")
}
