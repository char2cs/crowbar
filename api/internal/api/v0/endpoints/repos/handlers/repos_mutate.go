package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/domain/problem"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// patchRequest is the PATCH .../repos/:repoId body. Every field is optional and
// a nil field is left as it is. ProjectID may only name the repo's own project.
type patchRequest struct {
	Name      *string `json:"name"`
	ProjectID *string `json:"projectId"`
	Order     *int    `json:"order"`
	// FolderID re-files the repo's own entry within its project's home tree —
	// "" for the project's home root, a project-home folder id otherwise.
	// *string, not string, for the same reason ProjectID is: a request that
	// omits it must leave the repo's current folder untouched, not clear it.
	FolderID *string `json:"folderId"`
}

// Patch handles PATCH /v0/projects/:projectId/repos/:repoId: rename, sidebar
// reorder and re-filing into a home folder. It delivers the updated repo as a
// RepoDTO on the repos WebSocket stream so every client's sidebar refreshes.
//
// Validation is synchronous, and so is the write: none of the three is a git
// operation, so unlike Create this runs inline and answers 204 — the updated
// avatar and order ride the repos stream, not this response (the FE apiFetch throws
// on any non-enveloped 200 body, matching the icon mutations).
//
// A projectId naming another project is refused with 409: a repo's project is
// fixed at import (project.refuseProjectMove).
func (h *Handlers) Patch(
	c *gin.Context,
) {
	if h.updater == nil {
		libs.WriteErr(c, http.StatusInternalServerError, "repo update unavailable")
		return
	}
	var body patchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	update, ok := h.bindRepoUpdate(c, body)
	if !ok {
		return
	}
	if _, err := h.updater.UpdateRepo(c.Request.Context(), c.Param("repoId"), update); err != nil {
		libs.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// bindRepoUpdate validates the PATCH body into a project.RepoUpdate, writing the
// 400 and returning ok=false on any rejection. The name rules are the SAME ones
// create enforces, and for the same reason: a repo with no parseable remote falls
// back to its name for the on-disk worktree slug, so a separator or a dot-only
// component would derive worktrees outside the crowbar home.
func (h *Handlers) bindRepoUpdate(
	c *gin.Context,
	body patchRequest,
) (project.RepoUpdate, bool) {
	update := project.RepoUpdate{ProjectID: body.ProjectID, Order: body.Order, FolderID: body.FolderID}
	if body.Name == nil {
		return update, true
	}
	name := strings.TrimSpace(*body.Name)
	if name == "" {
		libs.WriteErr(c, http.StatusBadRequest, "name is required")
		return project.RepoUpdate{}, false
	}
	if !safeRepoName(name) {
		libs.WriteErr(c, http.StatusBadRequest, unsafeRepoNameMessage)
		return project.RepoUpdate{}, false
	}
	update.Name = &name
	return update, true
}

// DeleteRepo handles DELETE /v0/projects/:projectId/repos/:repoId. The intent
// is recorded and announced before the 202 (work at risk without consent is
// refused first), so boot finishes what this starts; the repo.delete operation
// says how it ended. The user's real repository directory is never touched.
func (h *Handlers) DeleteRepo(
	c *gin.Context,
) {
	projectID := c.Param("projectId")
	repoID := c.Param("repoId")
	if libs.Replayed(c, h.ops, domain.OpRepoDelete, domain.EntityRef{Kind: domain.EntityRepo, ID: repoID}) {
		return
	}
	repo, err := h.store.FindByKey(c.Request.Context(), repoID)
	if err != nil {
		libs.Fail(c, err)
		return
	}
	if repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	if h.deleter == nil {
		libs.WriteErr(c, http.StatusInternalServerError, "repo delete is not wired")
		return
	}
	consent := libs.DeleteConsentOf(c)
	marked, err := h.deleter.BeginRepoDelete(c.Request.Context(), *repo, consent)
	if err != nil {
		libs.Fail(c, err)
		return
	}
	ref := domain.EntityRef{Kind: domain.EntityRepo, ID: repoID}
	item := domain.OperationItem{Ref: ref, Label: marked.Name, State: domain.OpRunning}
	op, err := h.ops.Start(c.Request.Context(), operation.Spec{
		ID:        libs.IdempotencyKey(c),
		Kind:      domain.OpRepoDelete,
		Subject:   ref,
		ProjectID: projectID,
		Label:     "Remove " + marked.Name,
		Items:     []domain.OperationItem{item},
	}, func(ctx context.Context, p operation.Progress) error {
		if err := h.deleter.DeleteRepo(ctx, marked, consent); err != nil {
			failure := problem.Of(err)
			item.State, item.Error = domain.OpFailed, &failure
			p.Item(item)
			return err
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
