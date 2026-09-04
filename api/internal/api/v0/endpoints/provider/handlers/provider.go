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

// State handles GET /v0/chats/:chatId/provider (routes.go). Runs PollOnView
// for the resolved workspace and returns its ProviderState JSON. When
// capability is disabled, returns ProviderState{Protected: false, PR: nil}.
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

// workspace answers which workspace provider's State route acts on
// (routes.go): the chat group's resolveChatWorktree middleware has already
// resolved the chat's worktree and stashed it on the context, so it is read
// back from reqscope. A miss means the route is mounted outside that
// middleware, which is a wiring bug rather than anything the caller did.
// /protected-branches does not move (spec §4.2) and keeps its own repo-scoped
// resolution (worktreeForRepo below), untouched by this helper.
func (h *Handlers) workspace(
	ctx *gin.Context,
) (domain.Workspace, bool) {
	ws, ok := reqscope.Workspace(ctx)
	if !ok {
		libs.WriteErr(ctx, http.StatusInternalServerError, "chat worktree not resolved")
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
