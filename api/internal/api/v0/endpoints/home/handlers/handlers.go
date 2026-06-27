// Package handlers serves the /v0/projects/:projectId/home routes.
package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	fileusecase "github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// HomeReader resolves the home workspace for a project.
type HomeReader interface {
	GetHomeForProject(ctx context.Context, projectID string) (domain.Workspace, error)
}

// Files is the file usecase surface needed by home file handlers.
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

// TerminalEngine is the terminal engine surface needed by home terminal handlers.
type TerminalEngine interface {
	Create(
		ctx context.Context,
		workspaceID string,
		workspaceDir string,
		prof *domain.TerminalProfile,
	) (sessionID string, err error)
	Kill(
		ctx context.Context,
		sessionID string,
	) error
	ListSessionsForWorkspace(
		workspaceID string,
	) []string
}

// Handlers serves all /home/* routes.
type Handlers struct {
	reader  HomeReader
	files   Files
	termEng TerminalEngine
}

// New builds Handlers.
func New(reader HomeReader, files Files, termEng TerminalEngine) *Handlers {
	return &Handlers{reader: reader, files: files, termEng: termEng}
}

// resolveHome fetches the home workspace for the project named in the request
// path. It writes a 404 when the workspace is not found and a 500 for any
// other storage failure, returning false in both cases.
func (h *Handlers) resolveHome(c *gin.Context) (domain.Workspace, bool) {
	ws, err := h.reader.GetHomeForProject(c.Request.Context(), c.Param("projectId"))
	if err != nil {
		if errors.Is(err, apperr.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "home workspace not found")
		} else {
			libs.WriteErr(c, http.StatusInternalServerError, "failed to resolve home workspace")
		}
		return domain.Workspace{}, false
	}
	return ws, true
}
