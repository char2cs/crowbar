package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
)

// importRequest is the POST …/workspaces/import body: the branches to import as
// managed workspaces. Each is PR-parented up to a protected/default root, with
// missing ancestors created (see worktree.CreateFromImport).
type importRequest struct {
	Branches []string `json:"branches"`
}

// Import handles POST /v0/projects/:projectId/repos/:repoId/workspaces/import.
// It validates synchronously (repo present, branches non-empty), returns 202,
// and resolves + creates the tree in the background. Each created workspace
// arrives on the per-repo workspace WebSocket stream, exactly like a single
// create; a batch that fails before producing any workspace is best-effort
// logged (no entity to hang LastError on).
func (h *Handlers) Import(c *gin.Context) {
	var body importRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.Branches) == 0 {
		libs.WriteErr(c, http.StatusBadRequest, "branches is required")
		return
	}
	repo, err := h.repos.FindByKey(c.Request.Context(), c.Param("repoId"))
	if err != nil {
		status, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, status, msg)
		return
	}
	if repo == nil {
		libs.WriteErr(c, http.StatusNotFound, "repo not found")
		return
	}
	// The chain walk TERMINATES at the default branch, so importing it creates
	// nothing — and a 202 for work defined to do nothing is a silent hang: the
	// caller's optimistic row is cleared only by a workspace that never arrives.
	// Refuse it here, on the request path, where the caller can surface it.
	for _, b := range body.Branches {
		if b == repo.DefaultBranch {
			libs.WriteErr(c, http.StatusConflict, fmt.Sprintf(
				"%s is the repository's default branch — it is already the repo's home workspace", b))
			return
		}
	}
	in := worktree.ImportInput{
		RepoID:        repo.ID,
		ProjectID:     repo.ProjectID,
		RepoPath:      repo.Path,
		RemoteURL:     repo.RemoteURL,
		DefaultBranch: repo.DefaultBranch,
		Branches:      body.Branches,
	}
	libs.WriteAccepted(c)
	h.runAsync(
		c.Request.Context(),
		h.working,
		h.broadcastLastError,
		"",
		func(ctx context.Context) error {
			if importErr := h.hierarchy.CreateFromImport(ctx, in); importErr != nil {
				slog.WarnContext(ctx, "workspace import failed",
					"repo", in.RepoID, "branches", in.Branches, "err", importErr)
				return importErr
			}
			return nil
		},
	)
}
