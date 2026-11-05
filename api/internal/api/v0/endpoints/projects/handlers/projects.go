// Package handlers holds the gin handlers backing the projects endpoint.
package handlers

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
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
	// Update applies a partial project update: the display name, and the icon
	// fields the icon handlers below persist through.
	Update(
		ctx context.Context,
		projectID string,
		in project.Update,
	) (domain.Project, error)
}

// Importer is the import surface the projects handlers need: validate the path
// and persist the Project record, under the caller's id, with its home workspace.
type Importer interface {
	Create(
		ctx context.Context,
		id string,
		name string,
		path string,
	) (domain.Project, error)
}

// Deleter is the delete surface the projects handlers need: cascade-remove a
// project's records (workspaces, repos, then the project itself), tearing only
// crowbar-created worktree directories down on disk.
type Deleter interface {
	BeginDelete(
		ctx context.Context,
		id string,
		consent domain.DeleteConsent,
	) (domain.Project, error)
	Delete(
		ctx context.Context,
		id string,
		consent domain.DeleteConsent,
	) error
}

// Operations runs the work a create or a delete hands off after answering 202.
type Operations interface {
	Start(
		ctx context.Context,
		spec operation.Spec,
		work operation.Work,
	) (domain.Operation, error)
	libs.Replayer
}

// Handlers serves the /v0/projects routes from the project read, import, and
// delete usecases. A create or a delete checks what it can, answers 202 with
// its operation, and delivers the resulting ProjectDTO on the Projects stream.
type Handlers struct {
	reader   ListGetter
	importer Importer
	deleter  Deleter
	ops      Operations
	stat     func(string) (os.FileInfo, error)
	// Where the project's icon bytes live, as the repo handlers' equivalent is.
	crowbarHome func() (string, error)
}

// New builds the projects Handlers from the project usecases, the operations
// service and the crowbar-home resolver, which is required. The project store
// publishes every row they write.
func New(
	reader ListGetter,
	importer Importer,
	deleter Deleter,
	ops Operations,
	crowbarHome func() (string, error),
) *Handlers {
	if crowbarHome == nil {
		panic("projects: crowbarHome is required")
	}
	return &Handlers{
		reader:      reader,
		importer:    importer,
		deleter:     deleter,
		ops:         ops,
		stat:        os.Stat,
		crowbarHome: crowbarHome,
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
		libs.Fail(c, err)
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
		libs.Fail(c, err)
		return
	}
	libs.WriteQueryOK(c, dto.ProjectDTOFrom(project))
}

// patchRequest is the PATCH /v0/projects/:projectId body. Every field is
// optional; the ones present are the ones applied.
type patchRequest struct {
	// Order is the project's new index in the sidebar.
	Order *int `json:"order"`
	// Name is the project's new display label — the sidebar's inline rename.
	Name *string `json:"name"`
}

// Patch handles PATCH /v0/projects/:projectId: a sidebar reorder, a rename, or
// both. Each is one store write, so it runs inline and answers 204; the updated
// rows ride the Projects stream.
func (h *Handlers) Patch(
	c *gin.Context,
) {
	var body patchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Order == nil && body.Name == nil {
		libs.WriteErr(c, http.StatusBadRequest, "order or name is required")
		return
	}
	ctx := c.Request.Context()
	projectID := c.Param("projectId")

	if body.Name != nil && !h.rename(c, ctx, projectID, *body.Name) {
		return
	}

	if body.Order != nil {
		if _, err := h.reader.Reorder(ctx, projectID, *body.Order); err != nil {
			libs.Fail(c, err)
			return
		}
	}
	c.Status(http.StatusNoContent)
}

// Import handles POST /v0/projects. What it can check up front (body, name,
// path on disk) is a 4xx; then it answers 202 with the project.create
// operation, whose one item is the project, named by the request's key.
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
	// Named by the key, so a retry is the same subject and replays.
	ref := domain.EntityRef{Kind: domain.EntityProject, ID: libs.IdempotencyKey(c)}
	if ref.ID == "" {
		ref.ID = uuid.NewString()
	}
	item := domain.OperationItem{Ref: ref, Label: body.Name, State: domain.OpRunning}
	op, err := h.ops.Start(c.Request.Context(), operation.Spec{
		ID:        libs.IdempotencyKey(c),
		Kind:      domain.OpProjectCreate,
		Subject:   ref,
		ProjectID: ref.ID,
		Label:     "Add " + body.Name,
		Items:     []domain.OperationItem{item},
	}, func(ctx context.Context, p operation.Progress) error {
		if _, err := h.importer.Create(ctx, ref.ID, body.Name, body.Path); err != nil {
			return failItem(p, item, err)
		}
		item.State = domain.OpSucceeded
		p.Item(item)
		return nil
	})
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteOperation(c, op)
}

// Delete handles DELETE /v0/projects/:projectId. The intent is recorded before
// the 202 so boot finishes an interrupted delete; work at risk without consent is
// refused first. Repository directories are never deleted from disk.
func (h *Handlers) Delete(
	c *gin.Context,
) {
	id := c.Param("projectId")
	if libs.Replayed(c, h.ops, domain.OpProjectDelete, domain.EntityRef{Kind: domain.EntityProject, ID: id}) {
		return
	}
	consent := libs.DeleteConsentOf(c)
	marked, err := h.deleter.BeginDelete(c.Request.Context(), id, consent)
	if err != nil {
		libs.Fail(c, err)
		return
	}
	ref := domain.EntityRef{Kind: domain.EntityProject, ID: id}
	item := domain.OperationItem{Ref: ref, Label: marked.Name, State: domain.OpRunning}
	op, err := h.ops.Start(c.Request.Context(), operation.Spec{
		ID:        libs.IdempotencyKey(c),
		Kind:      domain.OpProjectDelete,
		Subject:   ref,
		ProjectID: id,
		Label:     "Remove " + marked.Name,
		Items:     []domain.OperationItem{item},
	}, func(ctx context.Context, p operation.Progress) error {
		if err := h.deleter.Delete(ctx, id, consent); err != nil {
			return failItem(p, item, err)
		}
		item.State = domain.OpSucceeded
		p.Item(item)
		return nil
	})
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteOperation(c, op)
}

// failItem records item as failed with err, and returns err for the operation.
func failItem(
	p operation.Progress,
	item domain.OperationItem,
	err error,
) error {
	failure := problem.Of(err)
	item.State, item.Error = domain.OpFailed, &failure
	p.Item(item)
	return err
}

// rename applies a project rename and reports whether the request may continue.
// It writes the error response itself on failure, so a false return means the
// handler is finished.
func (h *Handlers) rename(
	c *gin.Context,
	ctx context.Context,
	projectID string,
	raw string,
) bool {
	name := strings.TrimSpace(raw)
	if name == "" {
		libs.WriteErr(c, http.StatusBadRequest, "name must not be empty")
		return false
	}
	if _, err := h.reader.Update(ctx, projectID, project.Update{Name: &name}); err != nil {
		libs.Fail(c, err)
		return false
	}
	return true
}
