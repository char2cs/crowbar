package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

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
		slog.ErrorContext(ctx.Request.Context(), "provider: poll failed", "component", "provider", "workspace", ws.ID, "err", err)
		libs.WriteErr(ctx, http.StatusInternalServerError, "provider poll failed")
		return
	}

	libs.WriteQueryOK(ctx, state)
}

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
		slog.ErrorContext(ctx.Request.Context(), "provider: protected branches failed", "component", "provider", "repo", repoID, "err", err)
		libs.WriteErr(ctx, http.StatusInternalServerError, "provider poll failed")
		return
	}

	libs.WriteQueryOK(ctx, branches)
}

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
	return "", problem.ErrNotFound
}
