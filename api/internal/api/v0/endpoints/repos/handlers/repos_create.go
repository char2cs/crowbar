package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain/problem"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// createRequest is the POST .../repos body; the project is the URL's.
type createRequest struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Create handles POST /v0/projects/:projectId/repos. Up-front checks answer 4xx; then it answers
// 202 with the repo.create operation: one item for the repo, one per protected branch set up.
func (h *Handlers) Create(
	c *gin.Context,
) {
	var body createRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name == "" {
		libs.WriteErr(c, http.StatusBadRequest, "name is required")
		return
	}
	if !safeRepoName(body.Name) {
		libs.WriteErr(c, http.StatusBadRequest, unsafeRepoNameMessage)
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
	if h.importer == nil {
		libs.WriteErr(c, http.StatusInternalServerError, "repo import is not wired")
		return
	}
	in := project.RepoImport{ID: uuid.NewString(), ProjectID: c.Param("projectId"), Name: body.Name, Path: body.Path}
	item := domain.OperationItem{
		Ref: domain.EntityRef{Kind: domain.EntityRepo, ID: in.ID}, Label: "Add " + in.Name, State: domain.OpRunning,
	}
	op, err := h.ops.Start(c.Request.Context(), operation.Spec{
		ID:        libs.IdempotencyKey(c),
		Kind:      domain.OpRepoCreate,
		Subject:   domain.EntityRef{Kind: domain.EntityProject, ID: in.ProjectID},
		ProjectID: in.ProjectID,
		Label:     item.Label,
		Items:     []domain.OperationItem{item},
		// Only for a new key: a retry after the first add gets that operation back.
		Validate: func(ctx context.Context) error { return h.importer.CheckRepoImportable(ctx, in.ProjectID, in.Path) },
	}, func(ctx context.Context, p operation.Progress) error {
		return h.importRepo(ctx, in, item, p)
	})
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteOperation(c, op)
}

// importRepo is repo.create's work. The repo item names the repo the import
// returns: re-adding a folder this project has returns the existing one.
func (h *Handlers) importRepo(
	ctx context.Context,
	in project.RepoImport,
	item domain.OperationItem,
	p operation.Progress,
) error {
	repo, err := h.importer.ImportRepo(ctx, in, repoProgress{p})
	if err != nil {
		failure := problem.Of(err)
		item.State, item.Error = domain.OpFailed, &failure
		p.Item(item)
		return err
	}
	item.Ref.ID, item.State = repo.ID, domain.OpSucceeded
	p.Item(item)
	return nil
}

// repoProgress turns each protected branch's outcome into a workspace item.
type repoProgress struct {
	p operation.Progress
}

func (rp repoProgress) Workspace(id, branch string, err error) {
	item := domain.OperationItem{
		Ref: domain.EntityRef{Kind: domain.EntityWorkspace, ID: id}, Label: branch, State: domain.OpSucceeded,
	}
	if err != nil {
		failure := problem.Of(err)
		item.State, item.Error = domain.OpFailed, &failure
	}
	rp.p.Item(item)
}

// unsafeRepoNameMessage is the 400 both name-taking endpoints answer with.
const unsafeRepoNameMessage = "name must not contain a path separator or be a dot-only component"

// safeRepoName reports whether a client-supplied repo name may be an on-disk identity. A repo with
// no parseable remote uses its name as the worktree slug, so "../.." would place worktrees outside
// the crowbar home where no removal guard can reclaim them; create and rename both check it.
func safeRepoName(
	name string,
) bool {
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	return strings.Trim(name, ".") != ""
}
