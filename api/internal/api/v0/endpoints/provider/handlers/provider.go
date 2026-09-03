package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// State handles GET .../provider, on either of the two groups provider is
// currently mounted on (routes.go). Runs PollOnView for the resolved workspace
// and returns its ProviderState JSON. When capability is disabled, returns
// ProviderState{Protected: false, PR: nil}.
func (h *Handlers) State(
	ctx *gin.Context,
) {
	ws, ok := h.workspace(ctx)
	if !ok {
		return
	}

	pollCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()

	state, err := h.eng.PollOnView(
		pollCtx,
		ws.ID,
		ws.WorktreePath,
		ws.Branch,
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "provider: poll error for ws %s: %v\n", ws.ID, err)
		libs.WriteErr(ctx, http.StatusInternalServerError, "provider poll failed")
		return
	}

	libs.WriteQueryOK(ctx, state)
}

// workspace answers which workspace this request acts on, for either of the
// two groups provider's State route is currently mounted on (routes.go).
//
// On /v0/chats/:chatId/provider the chat group's resolveChatWorktree
// middleware has already resolved the chat's worktree and stashed the
// workspace on the context, so it is read back from reqscope — never resolved
// a second time per request. The :wsId branch serves the old workspace-scoped
// mount, unretired until spec §8 step 6; when that mount goes, so does the
// branch. /protected-branches does not move (spec §4.2) and keeps its own
// repo-scoped resolution (worktreeForRepo below), untouched by this helper.
//
// reqscope is consulted first because it is the resolved truth: the two
// mounts are disjoint, so exactly one source is ever populated.
func (h *Handlers) workspace(
	ctx *gin.Context,
) (domain.Workspace, bool) {
	if ws, ok := reqscope.Workspace(ctx); ok {
		return ws, true
	}
	ws, err := h.wsReader.Get(ctx.Request.Context(), ctx.Param("wsId"))
	if err != nil {
		libs.WriteErr(ctx, http.StatusNotFound, "workspace not found")
		return domain.Workspace{}, false
	}
	return ws, true
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
