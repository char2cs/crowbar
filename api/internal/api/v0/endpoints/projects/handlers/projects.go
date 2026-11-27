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
	Update(
		ctx context.Context,
		projectID string,
		in project.Update,
	) (domain.Project, error)
}

type Importer interface {
	Create(
		ctx context.Context,
		id string,
		name string,
		path string,
	) (domain.Project, error)
}

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

type Operations interface {
	Start(
		ctx context.Context,
		spec operation.Spec,
		work operation.Work,
	) (domain.Operation, error)
	libs.Replayer
}

type Handlers struct {
	reader      ListGetter
	importer    Importer
	deleter     Deleter
	ops         Operations
	stat        func(string) (os.FileInfo, error)
	crowbarHome func() (string, error)
}

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

func (h *Handlers) WithStat(
	stat func(string) (os.FileInfo, error),
) *Handlers {
	if stat != nil {
		h.stat = stat
	}
	return h
}

type importRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

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

type patchRequest struct {
	Order *int    `json:"order"`
	Name  *string `json:"name"`
}

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
