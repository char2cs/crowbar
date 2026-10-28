package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Push POST /v0/chats/:chatId/git/push answers 202 with the operation running
// the push; its outcome arrives on the operations stream.
func (h *Handlers) Push(
	ctx *gin.Context,
) {
	ws, _ := reqscope.Workspace(ctx)
	h.startGit(ctx, domain.OpGitPush, label("Push", ws.Branch), func(c context.Context) error {
		return h.git.Push(c, ws.ID, time.Now())
	})
}

// Fetch POST /v0/chats/:chatId/git/fetch answers 202 with its operation.
func (h *Handlers) Fetch(
	ctx *gin.Context,
) {
	ws, _ := reqscope.Workspace(ctx)
	h.startGit(ctx, domain.OpGitFetch, "Fetch", func(c context.Context) error {
		return h.git.Fetch(c, ws.ID, time.Now())
	})
}

// Pull POST /v0/chats/:chatId/git/pull answers 202 with its operation.
func (h *Handlers) Pull(
	ctx *gin.Context,
) {
	ws, _ := reqscope.Workspace(ctx)
	h.startGit(ctx, domain.OpGitPull, "Pull", func(c context.Context) error {
		return h.git.Pull(c, ws.ID, "", time.Now())
	})
}

// startGit starts run as an operation on the request's worktree and answers
// 202 with it, or the refusal.
func (h *Handlers) startGit(
	ctx *gin.Context,
	kind domain.OperationKind,
	label string,
	run func(ctx context.Context) error,
) {
	ws, _ := reqscope.Workspace(ctx)
	op, err := h.ops.Start(ctx.Request.Context(), operation.Spec{
		ID:        libs.IdempotencyKey(ctx),
		Kind:      kind,
		Subject:   domain.EntityRef{Kind: domain.EntityWorkspace, ID: ws.ID},
		ProjectID: ws.ProjectID,
		Label:     label,
	}, func(c context.Context, _ operation.Progress) error {
		return run(c)
	})
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	libs.WriteOperation(ctx, op)
}

func label(verb, ref string) string {
	return strings.TrimSpace(verb + " " + ref)
}
