// Package handlers holds the gin handlers backing the projects endpoint.
package handlers

import (
	"context"
	"net/http"
	"os"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ListGetter is the read surface the projects handlers need — list every
// project, fetch one by id — plus the one write that is not an import or a
// delete: the sidebar reorder.
type ListGetter interface {
	List(
		ctx context.Context,
	) ([]domain.Project, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Project, error)
	Reorder(
		ctx context.Context,
		projectID string,
		order int,
	) (domain.Project, error)
}

// Importer is the import surface the projects handlers need: validate the path
// and persist the Project record with its home workspace.
type Importer interface {
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
// delete usecases. Domain mutations follow the fail-fast/good-path-async pattern
// (00 §4): validate synchronously, return 202, run the slow work in the
// background, then deliver the resulting ProjectDTO on the Projects WebSocket
// stream via broadcast.
type Handlers struct {
	reader    ListGetter
	importer  Importer
	deleter   Deleter
	broadcast func(dto.ProjectDTO)
	stat      func(string) (os.FileInfo, error)
	// async tracks the detached runAsync ops so callers can block on their real
	// completion instead of guessing with a sleep (see runAsync / WaitAsync).
	async sync.WaitGroup
}

// New builds the projects Handlers from the project read, import, and delete
// usecases plus the Projects-channel broadcast func. A nil broadcast degrades to
// a no-op so the handler never panics when wired without a hub (tests, dormant
// surfaces).
func New(
	reader ListGetter,
	importer Importer,
	deleter Deleter,
	broadcast func(dto.ProjectDTO),
) *Handlers {
	if broadcast == nil {
		broadcast = func(dto.ProjectDTO) {}
	}
	return &Handlers{
		reader:    reader,
		importer:  importer,
		deleter:   deleter,
		broadcast: broadcast,
		stat:      os.Stat,
	}
}

// WithStat overrides the filesystem stat used to validate the import path
// synchronously. Intended for tests; a nil arg leaves os.Stat in place.
func (h *Handlers) WithStat(
	stat func(string) (os.FileInfo, error),
) *Handlers {
	if stat != nil {
		h.stat = stat
	}
	return h
}

// importRequest is the POST /v0/projects body: the display name and the
// filesystem path to import.
type importRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
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

// Detail handles GET /v0/projects/:projectId, returning a single ProjectDTO.
func (h *Handlers) Detail(
	c *gin.Context,
) {
	project, err := h.reader.Get(c.Request.Context(), c.Param("projectId"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteQueryOK(c, dto.ProjectDTOFrom(project))
}

// patchRequest is the PATCH /v0/projects/:projectId body: the project's new
// index in the sidebar.
type patchRequest struct {
	Order *int `json:"order"`
}

// Patch handles PATCH /v0/projects/:projectId. Reordering is the only project
// field a user edits from the sidebar, and unlike an import it is a single store
// write, so it runs inline and answers 204 — the new order rides the Projects WS
// stream, not this response.
//
// The reorder densifies the whole list, so every project it shifts is broadcast
// too: a client told only about the row that was dragged would hold stale orders
// for the rest until the next reconnect.
func (h *Handlers) Patch(
	c *gin.Context,
) {
	var body patchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Order == nil {
		libs.WriteErr(c, http.StatusBadRequest, "order is required")
		return
	}
	if _, err := h.reader.Reorder(c.Request.Context(), c.Param("projectId"), *body.Order); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	h.broadcastAll(c.Request.Context())
	c.Status(http.StatusNoContent)
}

// broadcastAll re-delivers every project after a reorder. The list is a handful
// of rows and the reorder already read all of them, so re-broadcasting the set
// is cheaper than tracking which ones the densify happened to shift.
func (h *Handlers) broadcastAll(
	ctx context.Context,
) {
	rows, err := h.reader.List(ctx)
	if err != nil {
		return
	}
	for _, row := range rows {
		h.broadcast(dto.ProjectDTOFrom(row))
	}
}

// Import handles POST /v0/projects. It validates the request synchronously (body
// shape, name present, path present and existing on disk) returning 4xx on any
// failure; then it returns 202 and runs the create in the background. On
// success the created project is delivered as a ProjectDTO on the Projects
// WebSocket stream.
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
	if _, err := h.stat(body.Path); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "path does not exist")
		return
	}
	libs.WriteAccepted(c)
	name := body.Name
	path := body.Path
	h.runAsync(c.Request.Context(), func(ctx context.Context) {
		project, err := h.importer.Create(ctx, name, path)
		if err != nil {
			return
		}
		h.broadcast(dto.ProjectDTOFrom(project))
	})
}

// Delete handles DELETE /v0/projects/:projectId. It validates the project exists
// synchronously (4xx if not), then returns 202 and runs the cascade delete in
// the background. A deleted-status ProjectDTO tombstone is broadcast on the
// Projects WebSocket stream after teardown so the client cache drops the entity
// (00 §6). Real repository directories are never deleted from disk; only
// crowbar-created worktree directories are torn down (see project.DeleteUsecase).
func (h *Handlers) Delete(
	c *gin.Context,
) {
	id := c.Param("projectId")
	if _, err := h.reader.Get(c.Request.Context(), id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteAccepted(c)
	h.runAsync(c.Request.Context(), func(ctx context.Context) {
		if err := h.deleter.Delete(ctx, id); err != nil {
			return
		}
		h.broadcast(dto.ProjectDTO{ID: id, Status: "deleted"})
	})
}
