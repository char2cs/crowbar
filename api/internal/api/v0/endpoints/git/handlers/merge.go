package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Merge POST /v0/chats/:chatId/git/merge is a slow git op: it validates the
// target branch synchronously (4xx) then returns 202 and runs the merge in the
// background.
func (h *Handlers) Merge(
	ctx *gin.Context,
) {
	wsID := h.workspaceID(ctx)

	var body struct {
		Branch string `json:"branch"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.Branch == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "branch is required")
		return
	}

	h.startGit(ctx, domain.OpGitMerge, label("Merge", body.Branch), func(c context.Context) error {
		return h.git.Merge(c, wsID, body.Branch, time.Now())
	})
}

// Rebase POST /v0/chats/:chatId/git/rebase is a slow git op: it validates the
// onto branch synchronously (4xx) then returns 202 and runs the rebase in the
// background.
func (h *Handlers) Rebase(
	ctx *gin.Context,
) {
	wsID := h.workspaceID(ctx)

	var body struct {
		Branch string `json:"branch"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.Branch == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "branch is required")
		return
	}

	h.startGit(ctx, domain.OpGitRebase, label("Rebase onto", body.Branch), func(c context.Context) error {
		return h.git.Rebase(c, wsID, body.Branch, time.Now())
	})
}

// ResolveHunk POST /v0/chats/:chatId/git/resolve-hunk
func (h *Handlers) ResolveHunk(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

	var body struct {
		Path            string `json:"path"`
		HunkIndex       int    `json:"hunkIndex"`
		Choice          string `json:"choice"`
		ResolvedContent string `json:"resolvedContent"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if body.Path == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "path is required")
		return
	}
	if body.Choice == "" {
		libs.WriteErr(ctx, http.StatusBadRequest, "choice is required")
		return
	}

	hunkID := strconv.Itoa(body.HunkIndex)
	resolution := gitdomain.ConflictResolution(body.Choice)
	if err := h.git.ResolveHunk(
		reqCtx,
		wsID,
		body.Path,
		hunkID,
		resolution,
		body.ResolvedContent,
		time.Now(),
	); err != nil {
		libs.Fail(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}

// OperationContinue POST /v0/chats/:chatId/git/operation/continue
func (h *Handlers) OperationContinue(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

	if err := h.git.OperationContinue(reqCtx, wsID, time.Now()); err != nil {
		libs.Fail(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}

// OperationAbort POST /v0/chats/:chatId/git/operation/abort
func (h *Handlers) OperationAbort(
	ctx *gin.Context,
) {
	reqCtx := ctx.Request.Context()
	wsID := h.workspaceID(ctx)

	if err := h.git.OperationAbort(reqCtx, wsID, time.Now()); err != nil {
		libs.Fail(ctx, err)
		return
	}

	ctx.Status(http.StatusOK)
}
