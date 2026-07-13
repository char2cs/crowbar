package handlers

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// createRequest is the POST .../workspaces body: the new branch name and an
// optional parent workspace id. The repository to fork is taken from the
// :projectId/:repoId path params, not the body. When parentId is empty the new
// workspace forks from the repository's default branch; otherwise it forks from
// the parent workspace's branch.
type createRequest struct {
	Branch   string `json:"branch"`
	ParentID string `json:"parentId"`
}

// Create handles
// POST /v0/projects/:projectId/repos/:repoId/workspaces. It validates the
// request synchronously (body shape, repoId present, branch present, repo
// exists, parent resolves) returning 4xx on any failure; then it returns 202
// and runs CreateChild in the background. The created workspace is delivered on
// the workspace WebSocket stream via the repository's broadcast callback; a
// failure surfaces as LastError on… nothing yet, since no workspace id exists
// pre-create, so the create error is best-effort logged by the background run.
func (h *Handlers) Create(
	c *gin.Context,
) {
	var body createRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	repoID := c.Param("repoId")
	if repoID == "" {
		libs.WriteErr(c, http.StatusBadRequest, "repoId is required")
		return
	}
	if body.Branch == "" {
		libs.WriteErr(c, http.StatusBadRequest, "branch is required")
		return
	}
	in, err := h.buildCreateInput(c.Request.Context(), repoID, body)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	dup, err := h.branchTaken(c.Request.Context(), in.RepoID, in.Branch)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	if dup {
		libs.WriteErr(c, http.StatusConflict, "a workspace already exists for this branch")
		return
	}
	libs.WriteAccepted(c)
	h.runAsync(
		c.Request.Context(),
		h.working,
		h.broadcastLastError,
		"",
		func(ctx context.Context) error {
			_, createErr := h.hierarchy.CreateChild(ctx, in)
			if createErr != nil {
				// A create that fails before producing a workspace id has no entity to
				// hang LastError on (broadcastLastError is a no-op for a blank id), so
				// the failure would otherwise vanish. Log it so the operator can see
				// why an accepted create never produced a workspace.
				slog.WarnContext(ctx, "workspace create failed before producing an id",
					"repo", in.RepoID, "branch", in.Branch, "err", createErr)
			}
			return createErr
		},
	)
}

// broadcastLastError records a failed background mutation on the workspace
// entity. A blank wsID (for example a create that never produced an id) is a
// no-op since there is no entity to attach the error to.
func (h *Handlers) broadcastLastError(
	ctx context.Context,
	wsID string,
	message string,
) {
	if wsID == "" {
		return
	}
	_, _ = h.lastErrors.SetLastError(ctx, wsID, message)
}

// branchTaken reports whether a non-deleted workspace already holds the branch
// in the repo, so the create handler can return 409 synchronously instead of
// letting the async CreateChild reject after the 202 (where the error is only
// best-effort logged). The usecase guard remains the authoritative backstop.
func (h *Handlers) branchTaken(ctx context.Context, repoID, branch string) (bool, error) {
	all, err := h.reader.List(ctx)
	if err != nil {
		return false, err
	}
	for _, w := range all {
		// Only Crowbar-managed (non-default) workspaces count: the default
		// workspace is the imported repo folder and must never block importing
		// its branch as a real managed workspace.
		if w.RepoID == repoID && w.Branch == branch && !w.IsDefault &&
			w.Status != domain.WorkspaceStatusDeleted {
			return true, nil
		}
	}
	return false, nil
}

func (h *Handlers) buildCreateInput(
	ctx context.Context,
	repoID string,
	body createRequest,
) (worktree.CreateChildInput, error) {
	repo, err := h.repos.FindByKey(ctx, repoID)
	if err != nil {
		return worktree.CreateChildInput{}, err
	}
	if repo == nil {
		return worktree.CreateChildInput{}, apperr.ErrNotFound
	}
	parentBranch := repo.DefaultBranch
	if body.ParentID != "" {
		parentBranch, err = h.resolveParentBranch(ctx, body.ParentID)
		if err != nil {
			return worktree.CreateChildInput{}, err
		}
	}
	return worktree.CreateChildInput{
		RepoID:       repo.ID,
		ProjectID:    repo.ProjectID,
		RepoPath:     repo.Path,
		RemoteURL:    repo.RemoteURL,
		Branch:       body.Branch,
		ParentID:     body.ParentID,
		ParentBranch: parentBranch,
	}, nil
}

func (h *Handlers) resolveParentBranch(
	ctx context.Context,
	parentID string,
) (string, error) {
	parent, err := h.reader.Get(ctx, parentID)
	if err != nil {
		return "", err
	}
	return parent.Branch, nil
}

// Delete handles
// DELETE /v0/projects/:projectId/repos/:repoId/workspaces/:wsId. It validates
// the workspace exists synchronously (4xx if not), then returns 202 and runs the
// cascade delete in the background (locked rows are skipped by the usecase). The
// repository broadcasts a deleted-status tombstone on the workspace WebSocket
// stream; a failure surfaces as LastError on the entity.
func (h *Handlers) Delete(
	c *gin.Context,
) {
	id := c.Param("wsId")
	if _, err := h.reader.Get(c.Request.Context(), id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteAccepted(c)
	h.runAsync(
		c.Request.Context(),
		h.working,
		h.broadcastLastError,
		id,
		func(ctx context.Context) error {
			return h.hierarchy.DeleteCascade(ctx, id)
		},
	)
}
