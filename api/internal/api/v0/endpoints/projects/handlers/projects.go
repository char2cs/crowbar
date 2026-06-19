// Package handlers holds the gin handlers backing the projects endpoint.
package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ListGetter is the read surface the projects handlers need: list every project
// and fetch one by id.
type ListGetter interface {
	List(
		ctx context.Context,
	) ([]domain.Project, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Project, error)
}

// Importer is the import surface the projects handlers need: adopt a directory
// tree as a new Project.
type Importer interface {
	Import(
		ctx context.Context,
		name string,
		path string,
	) (domain.Project, error)
	// Create is the lightweight variant: validate the path and persist the
	// Project record only — no repo discovery, no workspace stubs. Used by
	// the OOBE flow where the project-level welcome screen handles repo setup.
	Create(
		ctx context.Context,
		name string,
		path string,
	) (domain.Project, error)
}

// Deleter is the delete surface the projects handlers need: cascade-remove a
// project's records (workspaces, repos, then the project itself), tearing only
// crowbar-created worktree directories down on disk.
type Deleter interface {
	Delete(
		ctx context.Context,
		id string,
	) error
}

// Handlers serves the /v0/projects routes from the project read, import, and
// delete usecases.
type Handlers struct {
	reader   ListGetter
	importer Importer
	deleter  Deleter
}

// New builds the projects Handlers from the project read, import, and delete
// usecases.
func New(
	reader ListGetter,
	importer Importer,
	deleter Deleter,
) *Handlers {
	return &Handlers{
		reader:   reader,
		importer: importer,
		deleter:  deleter,
	}
}

// importRequest is the POST /v0/projects body: the display name and the
// filesystem path to import.
type importRequest struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	// Quick skips repo discovery and workspace stub creation. Use it from the
	// OOBE flow where repo setup is deferred to the project-level welcome screen.
	Quick bool   `json:"quick,omitempty"`
}

// List handles GET /v0/projects, returning every project as ProjectDTO[].
func (h *Handlers) List(
	c *gin.Context,
) {
	projects, err := h.reader.List(c.Request.Context())
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.ProjectDTOList(projects))
}

// Detail handles GET /v0/projects/:id, returning a single ProjectDTO.
func (h *Handlers) Detail(
	c *gin.Context,
) {
	project, err := h.reader.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.ProjectDTOFrom(project))
}

// Import handles POST /v0/projects, adopting a directory tree as a new Project
// and returning the created id.
func (h *Handlers) Import(
	c *gin.Context,
) {
	var body importRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name == "" {
		libs.WriteErr(c, http.StatusBadRequest, "name is required")
		return
	}
	if body.Path == "" {
		libs.WriteErr(c, http.StatusBadRequest, "path is required")
		return
	}
	var (
		project domain.Project
		err     error
	)
	if body.Quick {
		project, err = h.importer.Create(c.Request.Context(), body.Name, body.Path)
	} else {
		project, err = h.importer.Import(c.Request.Context(), body.Name, body.Path)
	}
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusCreated, project.ID)
}

// Delete handles DELETE /v0/projects/:id, removing the project record together
// with its repo and workspace records and returning the requested id. Real
// repository directories are never deleted from disk; only crowbar-created
// worktree directories are torn down (see project.DeleteUsecase).
func (h *Handlers) Delete(
	c *gin.Context,
) {
	id := c.Param("id")
	if err := h.deleter.Delete(c.Request.Context(), id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, id)
}
