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

// ProjectReader resolves a project by ID — used for lazy home provisioning.
type ProjectReader interface {
	FindByKey(ctx context.Context, id string) (*domain.Project, error)
}

// HomeWorkspaces is the workspace surface the home handlers need.
type HomeWorkspaces interface {
	GetHomeForProject(ctx context.Context, projectID string) (domain.Workspace, error)
	CreateHome(ctx context.Context, projectID, worktreePath string, now time.Time) (domain.Workspace, error)
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
	workspaces HomeWorkspaces
	projects   ProjectReader
	files      Files
	termEng    TerminalEngine
}

// New builds Handlers.
func New(workspaces HomeWorkspaces, projects ProjectReader, files Files, termEng TerminalEngine) *Handlers {
	return &Handlers{workspaces: workspaces, projects: projects, files: files, termEng: termEng}
}

// resolveHome fetches the home workspace for the project. If not yet
// provisioned (ErrNotFound), it looks up the project path and creates one
// lazily — supporting projects created before the home feature was introduced.
func (h *Handlers) resolveHome(c *gin.Context) (domain.Workspace, bool) {
	projectID := c.Param("projectId")
	ws, err := h.workspaces.GetHomeForProject(c.Request.Context(), projectID)
	if err == nil {
		return ws, true
	}
	if !errors.Is(err, apperr.ErrNotFound) {
		libs.WriteErr(c, http.StatusInternalServerError, "failed to resolve home workspace")
		return domain.Workspace{}, false
	}

	// Lazily provision: look up the project to get its path, then create.
	project, pErr := h.projects.FindByKey(c.Request.Context(), projectID)
	if pErr != nil || project == nil {
		libs.WriteErr(c, http.StatusNotFound, "project not found")
		return domain.Workspace{}, false
	}
	ws, cErr := h.workspaces.CreateHome(c.Request.Context(), projectID, project.Path, time.Now())
	if cErr != nil {
		libs.WriteErr(c, http.StatusInternalServerError, "failed to provision home workspace")
		return domain.Workspace{}, false
	}
	return ws, true
}
