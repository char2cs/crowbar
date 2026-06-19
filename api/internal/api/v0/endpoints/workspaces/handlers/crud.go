package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
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
// POST /v0/projects/:projectId/repos/:repoId/workspaces, creating a
// worktree-backed workspace and returning the created id with status 201. The
// repo is resolved from the :repoId path param.
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
	created, err := h.hierarchy.CreateChild(c.Request.Context(), in)
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusCreated, created.ID)
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
// DELETE /v0/projects/:projectId/repos/:repoId/workspaces/:wsId,
// cascade-deleting the workspace and
// its descendants (locked rows are skipped by the usecase) and returning the
// requested id.
func (h *Handlers) Delete(
	c *gin.Context,
) {
	id := c.Param("wsId")
	if err := h.hierarchy.DeleteCascade(c.Request.Context(), id); err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, id)
}
