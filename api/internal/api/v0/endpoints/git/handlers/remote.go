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

func (h *Handlers) Push(
	ctx *gin.Context,
) {
	ws, _ := reqscope.Workspace(ctx)
	h.startGit(ctx, domain.OpGitPush, label("Push", ws.Branch), func(c context.Context) error {
		return h.git.Push(c, ws.ID, time.Now())
	})
}

func (h *Handlers) Fetch(
	ctx *gin.Context,
) {
	ws, _ := reqscope.Workspace(ctx)
	h.startGit(ctx, domain.OpGitFetch, "Fetch", func(c context.Context) error {
		return h.git.Fetch(c, ws.ID, time.Now())
	})
}

func (h *Handlers) Pull(
	ctx *gin.Context,
) {
	ws, _ := reqscope.Workspace(ctx)
	h.startGit(ctx, domain.OpGitPull, "Pull", func(c context.Context) error {
		return h.git.Pull(c, ws.ID, "", time.Now())
	})
}

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
