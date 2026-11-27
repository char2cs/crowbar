package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func (h *Handlers) chatWorkspace(
	c *gin.Context,
) (domain.Workspace, bool) {
	ws, err := h.worktrees.Resolve(c.Request.Context(), c.Param("id"))
	if err != nil {
		libs.WriteErr(c, http.StatusNotFound, "chat not found")
		return domain.Workspace{}, false
	}
	return ws, true
}

func (h *Handlers) startOn(
	c *gin.Context,
	ws domain.Workspace,
	kind domain.OperationKind,
	label string,
	run func(ctx context.Context) error,
) {
	op, err := h.ops.Start(c.Request.Context(), operation.Spec{
		ID:        libs.IdempotencyKey(c),
		Kind:      kind,
		Subject:   domain.EntityRef{Kind: domain.EntityWorkspace, ID: ws.ID},
		ProjectID: ws.ProjectID,
		Label:     strings.TrimSpace(label),
	}, func(ctx context.Context, _ operation.Progress) error {
		return run(ctx)
	})
	if err != nil {
		libs.Fail(c, err)
		return
	}
	libs.WriteOperation(c, op)
}

func (h *Handlers) relLabel(
	ctx context.Context,
	head string,
	rel string,
	id string,
) string {
	all, err := h.reader.List(ctx)
	if err != nil {
		return head
	}
	for _, ws := range all {
		if ws.ID == id && ws.Branch != "" {
			return head + " " + rel + " " + ws.Branch
		}
	}
	return head
}

type renameBranchRequest struct {
	Branch string `json:"branch"`
}

func (h *Handlers) ChatRenameBranch(
	c *gin.Context,
) {
	var body renameBranchRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, err.Error())
		return
	}
	ws, ok := h.chatWorkspace(c)
	if !ok {
		return
	}
	if err := h.applyRename(c, ws.ID, &body.Branch); err != nil {
		return
	}
	libs.WriteMutationOK(c, http.StatusOK, c.Param("id"))
}
