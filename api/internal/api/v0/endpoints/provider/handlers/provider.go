package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
)

// State handles
// GET /v0/projects/:projectId/repos/:repoId/workspaces/:wsId/provider.
// Runs PollOnView for the workspace and returns its ProviderState JSON.
// When capability is disabled, returns ProviderState{Protected: false, PR: nil}.
func (h *Handlers) State(
	ctx *gin.Context,
) {
	wsID := ctx.Param("wsId")
	ws, err := h.wsReader.Get(ctx.Request.Context(), wsID)
	if err != nil {
		libs.WriteErr(ctx, http.StatusNotFound, "workspace not found")
		return
	}

	pollCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()

	state, err := h.eng.PollOnView(
		pollCtx,
		wsID,
		ws.WorktreePath,
		ws.Branch,
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "provider: poll error for ws %s: %v\n", wsID, err)
		libs.WriteErr(ctx, http.StatusInternalServerError, "provider poll failed")
		return
	}

	libs.WriteQueryOK(ctx, state)
}

// ProtectedBranches handles
// GET /v0/projects/:projectId/repos/:repoId/protected-branches.
// It resolves a worktree path for the repo by finding any workspace whose
// RepoID matches the :repoId path param, then returns the list of protected
// branch names. When capability is disabled, returns the
// DefaultProtectedBranches fallback. The repo aggregate gains its own on-disk
// path in W11/W12; until then the repo-scoped worktree lookup stands in.
func (h *Handlers) ProtectedBranches(
	ctx *gin.Context,
) {
	repoID := ctx.Param("repoId")
	worktreePath, err := h.worktreeForRepo(ctx.Request.Context(), repoID)
	if err != nil {
		libs.WriteErr(ctx, http.StatusNotFound, "repo not found")
		return
	}

	branches, err := h.eng.ProtectedBranches(ctx.Request.Context(), worktreePath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "provider: protected-branches error for repo %s: %v\n", repoID, err)
		libs.WriteErr(ctx, http.StatusInternalServerError, "provider poll failed")
		return
	}

	libs.WriteQueryOK(ctx, branches)
}

// worktreeForRepo returns the worktree path of any workspace belonging to
// repoID, the stand-in repo root for the protected-branch lookup.
func (h *Handlers) worktreeForRepo(
	ctx context.Context,
	repoID string,
) (string, error) {
	all, err := h.wsReader.List(ctx)
	if err != nil {
		return "", err
	}
	for _, ws := range all {
		if ws.RepoID == repoID {
			return ws.WorktreePath, nil
		}
	}
	return "", apperr.ErrNotFound
}
